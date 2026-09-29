package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/willbengtson/thelastcall/internal/booking"
)

// Dynamo is a single-table DynamoDB store.
//
// Two item shapes share the table:
//
//	reservation   PK = RES#<id>        SK = RES#<id>
//	bay-hour hold PK = NIGHT#<date>    SK = HOUR#<hh>#BAY#<n>
//
// A bay-hour item is the lock. Writing one under
// attribute_not_exists(PK) inside a transaction is what makes a hold atomic:
// either every hour of the stretch is ours or the whole write is rejected.
// tableWaitTimeout bounds the wait for a freshly created table to go active.
const tableWaitTimeout = 2 * time.Minute

type Dynamo struct {
	client *dynamodb.Client
	table  string
}

// NewDynamo wraps a DynamoDB client over one table.
func NewDynamo(client *dynamodb.Client, table string) *Dynamo {
	return &Dynamo{client: client, table: table}
}

var _ booking.Store = (*Dynamo)(nil)

func nightPK(date string) string      { return "NIGHT#" + date }
func resKey(id string) string         { return "RES#" + id }
func hourSK(hour, bay int) string     { return fmt.Sprintf("HOUR#%02d#BAY#%d", hour, bay) }
func s(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }
func n(v int) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.Itoa(v)}
}

// Occupied queries a night's partition for every claimed bay-hour.
func (d *Dynamo) Occupied(ctx context.Context, date string) (map[int]map[int]bool, error) {
	out := map[int]map[int]bool{}
	paginator := dynamodb.NewQueryPaginator(d.client, &dynamodb.QueryInput{
		TableName:              aws.String(d.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": s(nightPK(date)),
			":sk": s("HOUR#"),
		},
		ProjectionExpression: aws.String("SK"),
		ConsistentRead:       aws.Bool(true),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("querying night %s: %w", date, err)
		}
		for _, item := range page.Items {
			sk, ok := item["SK"].(*types.AttributeValueMemberS)
			if !ok {
				continue
			}
			hour, bay, err := parseHourSK(sk.Value)
			if err != nil {
				continue // an item shape we do not own
			}
			if out[hour] == nil {
				out[hour] = map[int]bool{}
			}
			out[hour][bay] = true
		}
	}
	return out, nil
}

// Hold writes the reservation and claims its bay-hours in one transaction.
func (d *Dynamo) Hold(ctx context.Context, r booking.Reservation) error {
	items := []types.TransactWriteItem{{
		Put: &types.Put{
			TableName: aws.String(d.table),
			Item: map[string]types.AttributeValue{
				"PK":        s(resKey(r.ID)),
				"SK":        s(resKey(r.ID)),
				"type":      s("reservation"),
				"id":        s(r.ID),
				"code":      s(r.Code),
				"date":      s(r.Date),
				"startHour": n(r.StartHour),
				"hours":     n(r.Hours),
				"bay":       n(r.Bay),
				"party":     n(r.Party),
				"guestName": s(r.GuestName),
				"contact":   s(r.Contact),
				"createdAt": s(r.CreatedAt.Format("2006-01-02T15:04:05Z07:00")),
			},
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		},
	}}
	for h := r.StartHour; h < r.StartHour+r.Hours; h++ {
		items = append(items, types.TransactWriteItem{
			Put: &types.Put{
				TableName: aws.String(d.table),
				Item: map[string]types.AttributeValue{
					"PK":            s(nightPK(r.Date)),
					"SK":            s(hourSK(h, r.Bay)),
					"type":          s("bayHour"),
					"reservationId": s(r.ID),
				},
				ConditionExpression: aws.String("attribute_not_exists(SK)"),
			},
		})
	}

	_, err := d.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: items,
	})
	if err == nil {
		return nil
	}
	var canceled *types.TransactionCanceledException
	if errors.As(err, &canceled) {
		for _, reason := range canceled.CancellationReasons {
			if aws.ToString(reason.Code) == "ConditionalCheckFailed" {
				return booking.ErrSlotTaken
			}
		}
	}
	return fmt.Errorf("transact hold: %w", err)
}

// Get loads one reservation.
func (d *Dynamo) Get(ctx context.Context, id string) (booking.Reservation, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"PK": s(resKey(id)),
			"SK": s(resKey(id)),
		},
	})
	if err != nil {
		return booking.Reservation{}, fmt.Errorf("getting reservation %s: %w", id, err)
	}
	if out.Item == nil {
		return booking.Reservation{}, booking.ErrNotFound
	}
	return itemToReservation(out.Item)
}

// Release deletes a reservation and frees every bay-hour it held.
func (d *Dynamo) Release(ctx context.Context, id string) error {
	r, err := d.Get(ctx, id)
	if err != nil {
		return err
	}
	items := []types.TransactWriteItem{{
		Delete: &types.Delete{
			TableName: aws.String(d.table),
			Key: map[string]types.AttributeValue{
				"PK": s(resKey(id)),
				"SK": s(resKey(id)),
			},
		},
	}}
	for h := r.StartHour; h < r.StartHour+r.Hours; h++ {
		items = append(items, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(d.table),
				Key: map[string]types.AttributeValue{
					"PK": s(nightPK(r.Date)),
					"SK": s(hourSK(h, r.Bay)),
				},
			},
		})
	}
	if _, err := d.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: items,
	}); err != nil {
		return fmt.Errorf("transact release: %w", err)
	}
	return nil
}

// EnsureTable creates the table if it is missing. Useful against
// DynamoDB Local; in production the table comes from infrastructure code.
func (d *Dynamo) EnsureTable(ctx context.Context) error {
	_, err := d.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(d.table),
	})
	if err == nil {
		return nil
	}
	var missing *types.ResourceNotFoundException
	if !errors.As(err, &missing) {
		return fmt.Errorf("describing table %s: %w", d.table, err)
	}

	if _, err := d.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(d.table),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
	}); err != nil {
		return fmt.Errorf("creating table %s: %w", d.table, err)
	}
	if err := dynamodb.NewTableExistsWaiter(d.client).Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(d.table),
	}, tableWaitTimeout); err != nil {
		return fmt.Errorf("waiting for table %s: %w", d.table, err)
	}
	return nil
}

func parseHourSK(sk string) (hour, bay int, err error) {
	// HOUR#<hh>#BAY#<n>
	parts := strings.Split(sk, "#")
	if len(parts) != 4 || parts[0] != "HOUR" || parts[2] != "BAY" {
		return 0, 0, fmt.Errorf("unrecognized sort key %q", sk)
	}
	if hour, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, fmt.Errorf("hour in %q: %w", sk, err)
	}
	if bay, err = strconv.Atoi(parts[3]); err != nil {
		return 0, 0, fmt.Errorf("bay in %q: %w", sk, err)
	}
	return hour, bay, nil
}

func itemToReservation(item map[string]types.AttributeValue) (booking.Reservation, error) {
	r := booking.Reservation{
		ID:        attrS(item, "id"),
		Code:      attrS(item, "code"),
		Date:      attrS(item, "date"),
		StartHour: attrN(item, "startHour"),
		Hours:     attrN(item, "hours"),
		Bay:       attrN(item, "bay"),
		Party:     attrN(item, "party"),
		GuestName: attrS(item, "guestName"),
		Contact:   attrS(item, "contact"),
	}
	if ts := attrS(item, "createdAt"); ts != "" {
		parsed, err := parseTime(ts)
		if err != nil {
			return booking.Reservation{}, fmt.Errorf("createdAt of %s: %w", r.ID, err)
		}
		r.CreatedAt = parsed
	}
	return r, nil
}

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339, s) }

func attrS(item map[string]types.AttributeValue, key string) string {
	if v, ok := item[key].(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

func attrN(item map[string]types.AttributeValue, key string) int {
	if v, ok := item[key].(*types.AttributeValueMemberN); ok {
		i, err := strconv.Atoi(v.Value)
		if err == nil {
			return i
		}
	}
	return 0
}
