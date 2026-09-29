// The JSON contract served by cmd/server. Field names mirror the Go structs.

export type Slot = {
  hour: number;
  label: string;
  taken: boolean;
};

export type Night = {
  date: string; // YYYY-MM-DD, the night's calendar date
  weekday: string; // "Sun"
  dayOfMonth: number;
  label: string; // "Tonight" or "Friday, Oct 2"
  closed: boolean;
  slots: Slot[];
};

export type Availability = {
  party: number;
  hours: number;
  bayCount: number;
  nights: Night[];
};

export type Reservation = {
  id: string;
  code: string;
  date: string;
  startHour: number;
  hours: number;
  bay: number;
  party: number;
  guestName?: string;
  contact?: string;
  createdAt: string;
};

export type BookingRequest = {
  date: string;
  startHour: number;
  hours: number;
  party: number;
  guestName?: string;
  contact?: string;
};

export type HouseHour = {
  weekday: string;
  closed: boolean;
  open?: string;
  lastCall?: string;
};

export type House = {
  name: string;
  timezone: string;
  bays: number;
  maxParty: number;
  nights: HouseHour[];
};

/** ApiError carries the guest-facing message the server chose, plus its status. */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }

  /** True when the bay filled up between rendering and booking. */
  get isConflict(): boolean {
    return this.status === 409;
  }
}

const BASE = "/api/v1";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${BASE}${path}`, {
      ...init,
      headers: {
        Accept: "application/json",
        ...(init?.body ? { "Content-Type": "application/json" } : {}),
        ...init?.headers,
      },
    });
  } catch (cause) {
    // A dropped connection reads the same as a dark room to the guest.
    throw new ApiError(0, "We could not reach the bar. Check your connection.");
  }

  if (!response.ok) {
    throw new ApiError(response.status, await errorMessage(response));
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

async function errorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { error?: unknown };
    if (typeof body.error === "string" && body.error.length > 0) {
      return body.error;
    }
  } catch {
    // Fall through to a generic line: a non-JSON body means a proxy or
    // gateway answered, not our server.
  }
  return "Something went wrong behind the bar. Try again.";
}

/** getAvailability reads the seven-night strip for a party size and duration. */
export function getAvailability(
  party: number,
  hours: number,
  signal?: AbortSignal,
): Promise<Availability> {
  const query = new URLSearchParams({ party: String(party), hours: String(hours) });
  return request<Availability>(`/availability?${query}`, signal ? { signal } : {});
}

/** getHouse reads the standing facts about the room. */
export function getHouse(signal?: AbortSignal): Promise<House> {
  return request<House>("/house", signal ? { signal } : {});
}

/** holdBay claims a bay. */
export function holdBay(req: BookingRequest): Promise<Reservation> {
  return request<Reservation>("/bookings", {
    method: "POST",
    body: JSON.stringify(req),
  });
}

/** releaseBay cancels a reservation. */
export function releaseBay(id: string): Promise<void> {
  return request<void>(`/bookings/${encodeURIComponent(id)}`, { method: "DELETE" });
}

/** isAbort reports whether a rejection is just a superseded request. */
export function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}
