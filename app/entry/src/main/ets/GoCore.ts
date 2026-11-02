// The Go core, typed: one place that knows the wire format is JSON, so the
// UI only sees objects. Everything here is a thin wrapper over libgocore.so
// (see entry/src/main/cpp/types/libgocore/index.d.ts).

import core from 'libgocore.so';
import { util } from '@kit.ArkTS';

/** One row of the list (core.Todo). */
export interface Todo {
  id: number;
  title: string;
  done: boolean;
}

/** One row of a big list (core.Row). */
export interface Row {
  id: number;
  title: string;
  done: boolean;
}

/** What the core makes of the accelerometer (core.LevelReading). */
export interface LevelReading {
  pitch: number;
  roll: number;
  rate: number;
  x: number;
  y: number;
  z: number;
}

/** What the core makes of a file (core.FileInfo). */
export interface FileInfo {
  name: string;
  bytes: number;
  kind: string;
  sha256: string;
}

/** A file inspection's answer: the file, or why not. */
export interface InspectResult {
  ok: boolean;
  file?: FileInfo;
  error?: string;
}

/** What the core tells the UI (core.Event): a new list, or a line of text. */
export interface Event {
  kind: string;
  todos?: Todo[];
  text?: string;
  level?: LevelReading;
}

function todosOf(json: string): Todo[] {
  try {
    const e = JSON.parse(json) as Event;
    return e.todos ?? [];
  } catch (err) {
    return [];
  }
}

export class GoCore {
  /**
   * Takes the core's events. Call it before start(): the core pushes the
   * list as soon as it has read it.
   */
  listen(fn: (e: Event) => void): void {
    core.listen((json: string) => {
      try {
        fn(JSON.parse(json) as Event);
      } catch (err) {
        // a malformed event isn't worth taking the UI down for
      }
    });
  }

  /** Starts the core: reads the list from filesDir, starts its goroutines. */
  start(filesDir: string): void {
    core.start(filesDir);
  }

  stop(): void {
    core.stop();
  }

  todos(): Todo[] {
    return todosOf(core.snapshot());
  }

  add(title: string): Todo[] {
    return todosOf(core.add(title));
  }

  toggle(id: number): Todo[] {
    return todosOf(core.toggle(id));
  }

  remove(id: number): Todo[] {
    return todosOf(core.remove(id));
  }

  remaining(): number {
    return core.remaining();
  }

  /**
   * Inspects a file the system opened for us (a photo from the picker): the
   * core reads it and reports its size, format and hash. Close the file here
   * — the core only borrows the descriptor.
   */
  inspect(fd: number, name: string): InspectResult {
    try {
      return JSON.parse(core.inspect(fd, name)) as InspectResult;
    } catch (err) {
      const out: InspectResult = { ok: false, error: 'bad answer from the core' };
      return out;
    }
  }

  /** How many rows the big list has. */
  rowsCount(): number {
    return core.rowsCount();
  }

  /**
   * A page of the big list, decoded from the compact form: the UI's own
   * parsing, of bytes the core laid out. No JSON, no intermediate strings.
   */
  rows(offset: number, limit: number): Row[] {
    const buf: ArrayBuffer = core.rows(offset, limit);
    return decodeRows(buf);
  }

  /** The same page as JSON (the comparison). */
  rowsJson(offset: number, limit: number): Row[] {
    try {
      const page = JSON.parse(core.rowsJson(offset, limit)) as RowsPage;
      return page.rows ?? [];
    } catch (err) {
      return [];
    }
  }

  /** The same page, still JSON: for measuring the parse on its own. */
  rowsJsonRaw(offset: number, limit: number): string {
    return core.rowsJson(offset, limit);
  }

  /** The same page, still bytes: for measuring the bridge on its own. */
  rowsRaw(offset: number, limit: number): ArrayBuffer {
    return core.rows(offset, limit);
  }

  /** Decodes the compact form. */
  decode(buf: ArrayBuffer): Row[] {
    return decodeRows(buf);
  }

  /** One accelerometer sample (m/s²): the sensor calls this ~50 times a second. */
  accel(x: number, y: number, z: number): void {
    core.accel(x, y, z);
  }

  /** The app is on screen (true) or not. */
  lifecycle(foreground: boolean): void {
    core.lifecycle(foreground ? 1 : 0);
  }
}

interface RowsPage {
  rows?: Row[];
}

const utf8: util.TextDecoder = util.TextDecoder.create('utf-8');

function decodeRows(buf: ArrayBuffer): Row[] {
  const view = new DataView(buf);
  const count = view.getUint32(0, true);
  const out: Row[] = [];
  let off = 4;
  for (let i = 0; i < count; i++) {
    const id = view.getUint32(off, true);
    off += 4;
    const done = view.getUint8(off) === 1;
    off += 1;
    const len = view.getUint16(off, true);
    off += 2;
    const title = utf8.decodeToString(new Uint8Array(buf, off, len));
    off += len;
    const row: Row = { id: id, title: title, done: done };
    out.push(row);
  }
  return out;
}

export const gocore: GoCore = new GoCore();
