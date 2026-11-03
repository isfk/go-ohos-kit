// The whole UI: plain ArkUI, plain ArkTS. Nothing here knows about Go beyond
// asking the core for data and listening for what it says has changed.

import { sensor } from '@kit.SensorServiceKit';
import { systemDateTime } from '@kit.BasicServicesKit';
import { CameraCard } from '../CameraCard';
import { DbCard } from '../DbCard';
import { LocationCard } from '../LocationCard';
import { PhotoCard } from '../PhotoCard';
import { gocore, Event, LevelReading, Row, Todo } from '../GoCore';

@Entry
@ComponentV2
struct Index {
  @Local todos: Todo[] = [];
  @Local draft: string = '';
  @Local remaining: number = 0;
  /** The core's goroutine pushing a line a second: the bridge, both ways. */
  @Local heartbeat: string = '';
  /** The compact wire against JSON, measured on this phone. */
  @Local bench: string = '';
  /** What a call across the bridge costs before it has done anything. */
  @Local bridge: string = '';
  /** The accelerometer, smoothed and turned into degrees by the core. */
  @Local level: LevelReading | undefined = undefined;
  @Local sensorError: string = '';

  aboutToAppear(): void {
    gocore.listen((e: Event) => {
      if (e.kind === 'todos') {
        this.todos = e.todos ?? [];
        this.remaining = gocore.remaining();
      } else if (e.kind === 'text') {
        this.heartbeat = e.text ?? '';
      } else if (e.kind === 'level') {
        this.level = e.level;
      }
    });
    const dir = getContext(this).filesDir;
    gocore.start(dir);
  }

  onPageShow(): void {
    gocore.lifecycle(true);
    this.startSensor();
  }

  onPageHide(): void {
    gocore.lifecycle(false);
    this.stopSensor();
  }

  /**
   * The accelerometer: the platform's callbacks, at 50 Hz, straight into Go.
   * The bridge passes three doubles and allocates nothing; the core smooths
   * and decides how often the UI hears about it.
   */
  private onAccel = (data: sensor.AccelerometerResponse): void => {
    gocore.accel(data.x, data.y, data.z);
  };

  private startSensor(): void {
    try {
      sensor.on(sensor.SensorId.ACCELEROMETER, this.onAccel, { interval: 20000000 }); // 20ms: 50Hz
      this.sensorError = '';
    } catch (err) {
      this.sensorError = `${err}`;
    }
  }

  private stopSensor(): void {
    try {
      sensor.off(sensor.SensorId.ACCELEROMETER, this.onAccel);
    } catch (err) {
      // already off
    }
  }

  private levelLine(): string {
    const l = this.level;
    if (this.sensorError.length > 0) {
      return this.sensorError;
    }
    if (l === undefined) {
      return 'waiting for the accelerometer…';
    }
    const signed = (v: number): string => (v >= 0 ? '+' : '') + v.toFixed(1);
    return `pitch ${signed(l.pitch)}° · roll ${signed(l.roll)}° · ${l.rate.toFixed(0)} samples/s`;
  }

  aboutToDisappear(): void {
    gocore.stop();
  }

  /**
   * The same 2000 rows twice: once as JSON (bytes → string → objects), once in
   * the compact form (bytes → objects). This is what the binary path is for.
   */
  private benchmark(): void {
    const page = 2000;
    const rounds = 10;
    let rows: Row[] = [];

    let start = Date.now();
    for (let i = 0; i < rounds; i++) {
      rows = gocore.rowsJson(0, page);
    }
    const jsonMs = (Date.now() - start) / rounds;
    const jsonBytes = gocore.rowsJsonRaw(0, page).length;

    start = Date.now();
    for (let i = 0; i < rounds; i++) {
      rows = gocore.rows(0, page);
    }
    const binMs = (Date.now() - start) / rounds;
    const binBytes = gocore.rowsRaw(0, page).byteLength;

    // And the wire on its own: no decoding at all, which is what a list that
    // only decodes the rows it shows actually pays.
    start = Date.now();
    for (let i = 0; i < rounds; i++) {
      gocore.rowsRaw(0, page);
    }
    const wireMs = (Date.now() - start) / rounds;

    const kb = 1024;
    const first: string = rows.length > 0 ? rows[0].title : '?';
    this.bench = `${gocore.rowsCount()} rows · ${page}: JSON ${jsonMs.toFixed(1)}ms/${(jsonBytes / kb).toFixed(0)}KB` +
      ` · binary ${binMs.toFixed(1)}ms/${(binBytes / kb).toFixed(0)}KB · wire only ${wireMs.toFixed(1)}ms` +
      ` · e.g. ${first}`;
  }

  /** Nanoseconds since boot: Date.now() is far too coarse for one call. */
  private nowNs(): number {
    return systemDateTime.getUptime(systemDateTime.TimeType.ACTIVE, true);
  }

  /**
   * What a call across the bridge costs when there is nothing in it — the
   * floor under every synchronous "ArkTS asks the core something" on the UI
   * thread. Three measurements, because three different things are in the way:
   *
   * - int back (remaining): NAPI call, cgo jump, C.int back. Pure overhead.
   * - string back (todos): the same plus CString malloc/free, the copy into a
   *   JS string, and JSON.parse. This is what a query result costs today.
   * - 300 KB in (analyze): overhead plus the buffer copy. The core reports its
   *   own maths time (micros), so total minus maths is the bridge's share.
   *
   * A few microseconds means a database query can be a synchronous call; a few
   * hundred means it has to go through the event channel instead.
   */
  private benchBridge(): void {
    const rounds = 20000;
    const warmup = 2000;
    for (let i = 0; i < warmup; i++) {
      gocore.remaining();
      gocore.todos();
    }

    // An int back: nothing allocated on either side.
    let start = this.nowNs();
    for (let i = 0; i < rounds; i++) {
      gocore.remaining();
    }
    const intNs = (this.nowNs() - start) / rounds;

    // A JSON string back, parsed: what a result actually costs today.
    start = this.nowNs();
    for (let i = 0; i < rounds; i++) {
      gocore.todos();
    }
    const strNs = (this.nowNs() - start) / rounds;

    // 300 KB in: overhead plus the copy, with the core's clock to separate
    // the maths from the crossing.
    const w = 240;
    const h = 320;
    const px = new ArrayBuffer(w * h * 4);
    const u8 = new Uint8Array(px);
    for (let i = 0; i < u8.length; i += 4) {
      const p = i >> 2;
      u8[i] = p & 0xFF;
      u8[i + 1] = (p >> 8) & 0xFF;
      u8[i + 2] = 128;
      u8[i + 3] = 255;
    }
    const pixRounds = 20;
    let micros = 0;
    start = this.nowNs();
    for (let i = 0; i < pixRounds; i++) {
      const res = gocore.analyze(px, w, h, 0);
      micros = res.image?.micros ?? 0;
    }
    const pixNs = (this.nowNs() - start) / pixRounds;
    const mathNs = micros * 1000;

    const us = (ns: number): string => (ns / 1000).toFixed(1);
    const kb = px.byteLength / 1024;
    this.bridge = `int ${intNs.toFixed(0)}ns · json+parse ${us(strNs)}µs` +
      ` · ${kb.toFixed(0)}KB in ${us(pixNs)}µs (math ${us(mathNs)}µs → bridge ${us(pixNs - mathNs)}µs)`;
    // Same numbers in the log: a screenshot is evidence, text is easier to cite.
    console.info(`gokit-bench bridge: ${this.bridge}`);
  }

  private add(): void {
    const title = this.draft.trim();
    if (title.length === 0) {
      return;
    }
    this.todos = gocore.add(title);
    this.remaining = gocore.remaining();
    this.draft = '';
  }

  build() {
    Column({ space: 12 }) {
      Row() {
        Text('Todos')
          .fontSize(32)
          .fontWeight(FontWeight.Bold)
        Blank()
        Text(this.heartbeat)
          .fontSize(13)
          .fontColor('#99000000')
      }
      .width('100%')
      .alignItems(VerticalAlign.Bottom)

      Row({ space: 8 }) {
        TextInput({ text: this.draft, placeholder: 'What needs doing?' })
          .layoutWeight(1)
          .onChange((v: string) => {
            this.draft = v;
          })
          .onSubmit(() => {
            this.add();
          })
        Button('Add')
          .enabled(this.draft.trim().length > 0)
          .onClick(() => {
            this.add();
          })
      }
      .width('100%')

      // A photo the platform takes and decodes, and the core measures.
      PhotoCard()

      // A platform stream with a subscription: where you are, and how far
      // you've walked since the first fix.
      LocationCard()

      // A platform callback at 50 Hz, into Go, and back out smoothed.
      Row({ space: 12 }) {
        // The bubble: where the core says the phone is tipped.
        Stack() {
          Circle()
            .width(22)
            .height(22)
            .fill('#0A59F7')
            // Clamped to the circle: the bubble stays inside it, as a level's does.
            .translate({
              x: this.level === undefined ? 0 : Math.max(-20, Math.min(20, this.level.roll)),
              y: this.level === undefined ? 0 : Math.max(-20, Math.min(20, -this.level.pitch)),
            })
        }
        .width(64)
        .height(64)
        .borderRadius(32)
        .backgroundColor('#0F000000')

        Text(this.levelLine())
          .fontSize(11)
          .fontColor('#99000000')
          .layoutWeight(1)
          .maxLines(2)
      }
      .width('100%')

      // A runtime permission, a native surface, and a session to tear down.
      CameraCard()

      // The wire format, measured rather than argued about.
      Row({ space: 12 }) {
        Button('Measure 20k')
          .fontSize(14)
          .backgroundColor('#1F0A59F7')
          .fontColor('#0A59F7')
          .onClick(() => {
            this.benchmark();
          })
        Text(this.bench.length > 0 ? this.bench : 'JSON or the compact form?')
          .fontSize(11)
          .fontColor('#99000000')
          .layoutWeight(1)
          .maxLines(3)
      }
      .width('100%')

      // The bridge on its own: what a call costs before it has done anything.
      Row({ space: 12 }) {
        Button('Bridge µs')
          .fontSize(14)
          .backgroundColor('#1F0A59F7')
          .fontColor('#0A59F7')
          .onClick(() => {
            this.benchBridge();
          })
        Text(this.bridge.length > 0 ? this.bridge : 'How expensive is a call?')
          .fontSize(11)
          .fontColor('#99000000')
          .layoutWeight(1)
          .maxLines(4)
      }
      .width('100%')

      // The same rows, written by two SQLite files: the core's and the
      // platform's. Measured rather than argued about.
      DbCard()

      List({ space: 4 }) {
        ForEach(this.todos, (t: Todo) => {
          ListItem() {
            Row({ space: 12 }) {
              Toggle({ type: ToggleType.Checkbox, isOn: t.done })
                .width(24)
                .height(24)
                .onChange(() => {
                  this.todos = gocore.toggle(t.id);
                  this.remaining = gocore.remaining();
                })
              Text(t.title)
                .fontSize(17)
                .layoutWeight(1)
                .decoration({
                  type: t.done ? TextDecorationType.LineThrough : TextDecorationType.None,
                  color: '#99000000',
                })
              Button('Delete')
                .fontSize(13)
                .height(30)
                .backgroundColor('#00000000')
                .fontColor('#E84026')
                .onClick(() => {
                  this.todos = gocore.remove(t.id);
                  this.remaining = gocore.remaining();
                })
            }
            .width('100%')
            .padding({ top: 8, bottom: 8 })
          }
        }, (t: Todo) => t.id.toString())
      }
      .layoutWeight(1)
      .width('100%')
      .divider({ strokeWidth: 0.5, color: '#1F000000', startMargin: 0, endMargin: 0 })

      Text(this.remaining === 1 ? '1 thing left' : `${this.remaining} things left`)
        .fontSize(13)
        .fontColor('#99000000')
        .width('100%')
    }
    .width('100%')
    .height('100%')
    .padding(16)
  }
}
