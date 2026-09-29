// Package config reads the server's settings from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is everything the server needs to know at startup.
type Config struct {
	Addr string

	// Store is "memory" or "dynamo".
	Store string

	// DynamoTable is the single table all items live in.
	DynamoTable string
	// DynamoEndpoint points at DynamoDB Local when set; empty means real AWS.
	DynamoEndpoint string
	// AWSRegion is the region for the DynamoDB client.
	AWSRegion string
	// AutoCreateTable creates the table on boot. For local dev only.
	AutoCreateTable bool

	// Bays is how many sim bays the house runs.
	Bays int
	// MaxParty is the largest party one bay seats.
	MaxParty int
	// BookingWindow is how many nights ahead guests can hold a bay.
	BookingWindow int
	// BookingLead is how much notice the bar needs before a tee time.
	BookingLead time.Duration

	// Timezone is the house's clock. Nights and last call are read in it.
	Timezone string

	// AllowedOrigins are the browser origins permitted to call the API.
	// Empty means same-origin only, which is what the embedded build uses.
	AllowedOrigins []string
}

// Load reads configuration from the environment, applying house defaults.
func Load() (Config, error) {
	c := Config{
		Addr:            env("TLC_ADDR", ":8080"),
		Store:           env("TLC_STORE", "memory"),
		DynamoTable:     env("TLC_DYNAMO_TABLE", "thelastcall"),
		DynamoEndpoint:  env("TLC_DYNAMO_ENDPOINT", ""),
		AWSRegion:       env("AWS_REGION", "us-west-2"),
		AutoCreateTable: envBool("TLC_DYNAMO_AUTOCREATE", false),
		Timezone:        env("TLC_TIMEZONE", "America/Los_Angeles"),
	}

	var err error
	if c.Bays, err = envInt("TLC_BAYS", 3); err != nil {
		return Config{}, err
	}
	if c.MaxParty, err = envInt("TLC_MAX_PARTY", 6); err != nil {
		return Config{}, err
	}
	if c.BookingWindow, err = envInt("TLC_BOOKING_WINDOW_DAYS", 7); err != nil {
		return Config{}, err
	}
	if c.BookingLead, err = envDuration("TLC_BOOKING_LEAD", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if origins := env("TLC_ALLOWED_ORIGINS", ""); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.AllowedOrigins = append(c.AllowedOrigins, o)
			}
		}
	}

	switch c.Store {
	case "memory", "dynamo":
	default:
		return Config{}, fmt.Errorf("TLC_STORE must be \"memory\" or \"dynamo\", got %q", c.Store)
	}
	return c, nil
}

// Location resolves the house timezone.
func (c Config) Location() (*time.Location, error) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("TLC_TIMEZONE %q: %w", c.Timezone, err)
	}
	return loc, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return v, nil
}

func envBool(key string, fallback bool) bool {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration like \"30m\": %w", key, err)
	}
	return v, nil
}
