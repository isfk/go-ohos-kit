// The whole UI: plain ArkUI, plain ArkTS. Nothing here knows about Go beyond
// asking the core for data and listening for what it says has changed.

import { sensor } from '@kit.SensorServiceKit';
import { photoAccessHelper } from '@kit.MediaLibraryKit';
import { fileIo as fs } from '@kit.CoreFileKit';
import { CameraCard } from '../CameraCard';
import { gocore, Event, FileInfo, LevelReading, Row, Todo } from '../GoCore';

@Entry
@Component
struct Index {
  @State todos: Todo[] = [];
  @State draft: string = '';
  @State remaining: number = 0;
  /** The core's goroutine pushing a line a second: the bridge, both ways. */
  @State heartbeat: string = '';
  /** A photo the system opened, as the core reads it. */
  @State photo: FileInfo | undefined = undefined;
  @State photoError: string = '';
  /** The compact wire against JSON, measured on this phone. */
  @State bench: string = '';
  /** The accelerometer, smoothed and turned into degrees by the core. */
  @State level: LevelReading | undefined = undefined;
  @State sensorError: string = '';

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
   * A platform capability the app doesn't write itself: the system's photo
   * picker hands back a URI, we open it and hand the descriptor to Go, which
   * reads it and works out what it is. The UI never sees the bytes.
   */
  private async pickPhoto(): Promise<void> {
    this.photoError = '';
    try {
      const picker = new photoAccessHelper.PhotoViewPicker();
      const chosen = await picker.select({
        MIMEType: photoAccessHelper.PhotoViewMIMETypes.IMAGE_TYPE,
        maxSelectNumber: 1,
      });
      const uri: string = chosen.photoUris.length > 0 ? chosen.photoUris[0] : '';
      if (uri.length === 0) {
        return;
      }
      const file = fs.openSync(uri, fs.OpenMode.READ_ONLY);
      const parts = uri.split('/');
      const name: string = parts.length > 0 ? parts[parts.length - 1] : 'photo';
      const result = gocore.inspect(file.fd, name);
      fs.closeSync(file);
      if (result.ok) {
        this.photo = result.file;
      } else {
        this.photoError = result.error ?? 'the core could not read it';
      }
    } catch (err) {
      this.photoError = `${err}`;
    }
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

  private photoLine(): string {
    const p = this.photo;
    if (p === undefined) {
      return this.photoError.length > 0 ? this.photoError : 'no photo yet';
    }
    const mb = (p.bytes / (1024 * 1024)).toFixed(1);
    return `${p.kind} · ${mb} MB · ${p.sha256.substring(0, 12)}…`;
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

      // A capability the UI asks the platform for, and the core does the work.
      Row({ space: 12 }) {
        Button('Check a photo')
          .fontSize(14)
          .backgroundColor('#1F0A59F7')
          .fontColor('#0A59F7')
          .onClick(() => {
            this.pickPhoto();
          })
        Text(this.photoLine())
          .fontSize(12)
          .fontColor('#99000000')
          .layoutWeight(1)
          .maxLines(2)
      }
      .width('100%')

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
