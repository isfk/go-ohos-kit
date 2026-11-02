/**
 * The Go core, as ArkTS sees it (libgocore.so, built from ../go).
 *
 * Calls are synchronous and run on the main thread: keep them small. The
 * core's own goroutines talk back through listen().
 */

export interface Todo {
  id: number;
  title: string;
  done: boolean;
}

/** Calls the core's events: a new list, or a line of text. */
export function listen(fn: (json: string) => void): void;

/** Starts the core: reads the list from filesDir, starts its goroutines. */
export function start(filesDir: string): void;

/** Stops the core's goroutines. */
export function stop(): void;

/** The list, as JSON (a snapshot event). */
export function snapshot(): string;

/** Adds a todo; returns the list after it. */
export function add(title: string): string;

/** Turns a todo done, or not; returns the list after it. */
export function toggle(id: number): string;

/** Removes a todo; returns the list after it. */
export function remove(id: number): string;

/** How many are left to do. */
export function remaining(): number;

/** What a file the system opened is: `{ok, file}` or `{ok:false, error}`. */
export function inspect(fd: number, name: string): string;

/** One location fix in; the track so far out: `{ok, point}` or `{ok:false, error}`. */
export function location(lat: number, lon: number, accuracy: number): string;

/** Forgets the track (the UI stopped listening). */
export function locationReset(): void;

/**
 * What a decoded picture looks like: `{ok, image}` or `{ok:false, error}`.
 * pixels is RGBA_8888, width×height, row stride in bytes (0 = packed).
 */
export function analyze(pixels: ArrayBuffer, width: number, height: number, stride?: number): string;

/** The app moved on screen (1) or to the background (0). */
export function lifecycle(foreground: number): void;

/** How many rows the big list has. */
export function rowsCount(): number;

/**
 * A page of the big list, in the compact form: one ArrayBuffer, no parsing.
 * u32 count, then per row u32 id, u8 done, u16 title length, title bytes.
 */
export function rows(offset: number, limit: number): ArrayBuffer;

/** The same page as JSON, to measure the compact form against. */
export function rowsJson(offset: number, limit: number): string;

/**
 * One accelerometer sample, in m/s² (gravity included). Call it from the
 * sensor's callback: it does no JavaScript work and allocates nothing.
 */
export function accel(x: number, y: number, z: number): void;
