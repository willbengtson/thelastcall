import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Masthead, Screen } from "../components/Chrome";
import {
  ApiError,
  getAvailability,
  holdBay,
  isAbort,
  releaseBay,
  type Availability,
  type Night,
  type Reservation,
} from "../lib/api";

const PARTY_SIZES = [1, 2, 3, 4, 5, 6] as const;
const DURATIONS = [1, 2, 3] as const;

/**
 * BookABay is the `Book a Bay` screen from the design, driven by the live
 * booking API instead of the design's stubbed-out hours.
 */
export function BookABay() {
  const [party, setParty] = useState(2);
  const [hours, setHours] = useState(1);

  // `date` and `hour` are the guest's preferences. What is actually selected is
  // derived from them against fresh availability, so a night going dark or a
  // tee time filling up quietly drops the selection instead of booking it.
  const [date, setDate] = useState<string | null>(null);
  const [hour, setHour] = useState<number | null>(null);

  const [availability, setAvailability] = useState<Availability | null>(null);
  const [loading, setLoading] = useState(true);
  const [trouble, setTrouble] = useState<string | null>(null);
  const [reservation, setReservation] = useState<Reservation | null>(null);
  const [working, setWorking] = useState(false);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    getAvailability(party, hours, controller.signal)
      .then((next) => {
        setAvailability(next);
        setTrouble(null);
      })
      .catch((error: unknown) => {
        if (isAbort(error)) return;
        setTrouble(error instanceof ApiError ? error.message : "We could not read the book.");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [party, hours, reloadToken]);

  const nights = availability?.nights ?? [];
  const night = useMemo(() => selectNight(nights, date), [nights, date]);
  const slot = night?.slots.find((s) => s.hour === hour && !s.taken) ?? null;

  const summary = useMemo(() => {
    const players = `${party} ${party === 1 ? "player" : "players"}`;
    const when = night?.label ?? "Some night";
    const stretch = `${hours} hr`;
    return slot
      ? `${when} at ${slot.label} · ${players} · ${stretch}`
      : `${when} · ${players} · ${stretch} — pick a tee time`;
  }, [night, slot, party, hours]);

  const pickNight = useCallback((picked: Night) => {
    setDate(picked.date);
    setHour(null);
  }, []);

  const pickDuration = useCallback((picked: number) => {
    // A longer stretch reshuffles the tee times, so the old pick cannot stand.
    setHours(picked);
    setHour(null);
  }, []);

  async function confirm() {
    if (!night || !slot) return;
    setWorking(true);
    setTrouble(null);
    try {
      setReservation(
        await holdBay({ date: night.date, startHour: slot.hour, hours, party }),
      );
    } catch (error: unknown) {
      setTrouble(
        error instanceof ApiError ? error.message : "We could not hold that bay.",
      );
      // Someone beat us to it, or the night moved on: re-read the book so the
      // grid stops offering what is gone.
      setHour(null);
      setReloadToken((n) => n + 1);
    } finally {
      setWorking(false);
    }
  }

  async function changeBooking() {
    const held = reservation;
    if (!held) return;
    setWorking(true);
    setTrouble(null);
    try {
      await releaseBay(held.id);
    } catch (error: unknown) {
      // A hold we cannot drop is worth saying out loud — it is still on the
      // book — but the guest still gets back to the grid.
      setTrouble(
        error instanceof ApiError
          ? `${held.code} may still be on the book: ${error.message}`
          : `${held.code} may still be on the book.`,
      );
    } finally {
      setReservation(null);
      setHour(null);
      setReloadToken((n) => n + 1);
      setWorking(false);
    }
  }

  if (reservation) {
    return (
      <Screen>
        <Masthead eyebrow="Sim bays" headline="Your bay is held" />
        <p className="lede rise">
          {summaryOf(reservation, night)}. Bring the password. We&apos;ll bring the rest.
        </p>
        <Ticket reservation={reservation} />
        {trouble && (
          <p className="notice notice--trouble" role="alert">
            {trouble}
          </p>
        )}
        <div className="actions">
          <Link to="/find-us" className="btn-outline">
            How to find us
          </Link>
          <Link to="/knock" className="btn-gold">
            No password yet?
          </Link>
        </div>
        <button type="button" className="btn-quiet" onClick={changeBooking} disabled={working}>
          {working ? "One moment" : "Change booking"}
        </button>
      </Screen>
    );
  }

  return (
    <Screen>
      <Masthead eyebrow="Sim bays" headline="Claim a bay" />
      <p className="lede">
        The fairway never closes. The bar does. Pick your night and we&apos;ll have a bay
        and a pour waiting.
      </p>

      {trouble && (
        <p className="notice notice--trouble" role="alert">
          {trouble}
        </p>
      )}

      {loading && !availability ? (
        <p className="notice" aria-live="polite">
          Checking the book…
        </p>
      ) : (
        <div className={`booking${loading ? " is-waiting" : ""}`}>
          <fieldset className="field">
            <legend className="field__label">Night</legend>
            <div className="grid-nights">
              {nights.map((candidate) => (
                <button
                  key={candidate.date}
                  type="button"
                  className={chipClass("chip-night", candidate.date === night?.date)}
                  disabled={candidate.closed}
                  aria-pressed={candidate.date === night?.date}
                  onClick={() => pickNight(candidate)}
                >
                  <span className="chip-night__dow">
                    {candidate.label === "Tonight" ? "Tonight" : candidate.weekday}
                  </span>
                  <span className="chip-night__date">{candidate.dayOfMonth}</span>
                  <span className="chip-night__note">
                    {candidate.closed ? "Lights off" : ""}
                  </span>
                </button>
              ))}
            </div>
          </fieldset>

          <div className="field-pair">
            <fieldset className="field">
              <legend className="field__label">Players</legend>
              <div className="grid-party">
                {PARTY_SIZES.map((n) => (
                  <button
                    key={n}
                    type="button"
                    className={chipClass("chip-party", n === party)}
                    aria-pressed={n === party}
                    aria-label={`${n} ${n === 1 ? "player" : "players"}`}
                    onClick={() => setParty(n)}
                  >
                    {n}
                  </button>
                ))}
              </div>
            </fieldset>

            <fieldset className="field">
              <legend className="field__label">How long</legend>
              <div className="grid-duration">
                {DURATIONS.map((n) => (
                  <button
                    key={n}
                    type="button"
                    className={chipClass("chip-duration", n === hours)}
                    aria-pressed={n === hours}
                    onClick={() => pickDuration(n)}
                  >
                    {n} hr
                  </button>
                ))}
              </div>
            </fieldset>
          </div>

          <fieldset className="field">
            <legend className="field__label">Tee time</legend>
            {night && night.slots.length > 0 ? (
              <div className="grid-slots">
                {night.slots.map((candidate) => (
                  <button
                    key={candidate.hour}
                    type="button"
                    className={chipClass("chip-slot", candidate.hour === slot?.hour)}
                    disabled={candidate.taken}
                    aria-pressed={candidate.hour === slot?.hour}
                    onClick={() => setHour(candidate.hour)}
                  >
                    {candidate.label}
                  </button>
                ))}
              </div>
            ) : (
              <p className="quiet-note">
                {night ? "Nothing left that long tonight" : "The lights are off all week"}
              </p>
            )}
          </fieldset>

          <div className="confirm-row">
            <span className="summary" aria-live="polite">
              {summary}
            </span>
            <button
              type="button"
              className="btn-gold"
              onClick={confirm}
              disabled={!slot || working}
            >
              {working ? "Holding…" : "Hold my bay"}
            </button>
          </div>
        </div>
      )}
    </Screen>
  );
}

/** Ticket is the stub the guest reads back at the door. */
function Ticket({ reservation }: { reservation: Reservation }) {
  return (
    <div className="ticket">
      <div className="ticket__cell">
        <span className="ticket__label">Bay</span>
        <span className="ticket__value">{reservation.bay}</span>
      </div>
      <div className="ticket__cell">
        <span className="ticket__label">Tee time</span>
        <span className="ticket__value">{formatHour(reservation.startHour)}</span>
      </div>
      <div className="ticket__cell">
        <span className="ticket__label">Under</span>
        <span className="ticket__value">{reservation.code}</span>
      </div>
    </div>
  );
}

/**
 * selectNight resolves the guest's preferred date against live availability,
 * falling back to the first night the lights are on.
 */
function selectNight(nights: Night[], preferred: string | null): Night | null {
  const chosen = nights.find((n) => n.date === preferred && !n.closed);
  return chosen ?? nights.find((n) => !n.closed) ?? null;
}

function chipClass(variant: string, selected: boolean): string {
  return `chip ${variant}${selected ? " chip--on" : ""}`;
}

/** summaryOf describes a held reservation, matching the picker's phrasing. */
function summaryOf(reservation: Reservation, night: Night | null): string {
  const players = `${reservation.party} ${reservation.party === 1 ? "player" : "players"}`;
  const when = night?.date === reservation.date ? night.label : longDate(reservation.date);
  return `${when} at ${formatHour(reservation.startHour)} · ${players} · ${reservation.hours} hr`;
}

/** formatHour renders a night hour the way the bar says it: 25 is 1AM. */
function formatHour(hour: number): string {
  const h = ((hour % 24) + 24) % 24;
  const suffix = h >= 12 ? "PM" : "AM";
  return `${h % 12 || 12}${suffix}`;
}

function longDate(date: string): string {
  const [year, month, day] = date.split("-").map(Number);
  if (!year || !month || !day) return date;
  return new Date(year, month - 1, day).toLocaleDateString("en-US", {
    weekday: "long",
    month: "short",
    day: "numeric",
  });
}
