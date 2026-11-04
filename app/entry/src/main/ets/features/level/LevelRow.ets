// The accelerometer, end to end: the platform's 50 Hz callback goes straight into
// Go, comes back smoothed and turned into degrees, and is drawn as a level.
//
// The subscription and the sensor belong to this component, not to the page:
// a page that owns a 50 Hz stream starts bleeding responsibilities. The two
// things that decide whether it runs are @Param active (the page is on screen)
// and AppLife (the app is in the foreground) — the same pattern LocationCard
// uses, and the reason Home doesn't leave the accelerometer running.

import { sensor } from '@kit.SensorServiceKit';
import { Event, gocore, LevelReading } from '../../core/GoCore';
import { onCoreEvent } from '../../core/events';
import { AppLife, appLife } from '../../state/AppLife';

@ComponentV2
export struct LevelRow {
  /** The page is showing. Index sets it from onPageShow/onPageHide. */
  @Param active: boolean = false;
  @Local private level: LevelReading | undefined = undefined;
  @Local private error: string = '';
  /** Ability-level foreground, which is what catches the Home key. */
  @Local private life: AppLife = appLife();
  private off: () => void = (): void => {};
  private running: boolean = false;

  aboutToAppear(): void {
    this.off = onCoreEvent('level', (e: Event): void => {
      this.level = e.level;
    });
    this.sync();
  }

  aboutToDisappear(): void {
    this.off();
    this.stop();
  }

  @Monitor('active')
  onActiveChanged(): void {
    this.sync();
  }

  @Monitor('life.foreground')
  onForegroundChanged(): void {
    this.sync();
  }

  private sync(): void {
    if (this.active && this.life.foreground) {
      this.start();
    } else {
      this.stop();
    }
  }

  /**
   * One callback per sample, 20 ms apart, into Go. The bridge passes three
   * doubles and allocates nothing; the core decides how often the UI hears back.
   */
  private onAccel = (data: sensor.AccelerometerResponse): void => {
    gocore.accel(data.x, data.y, data.z);
  };

  private start(): void {
    if (this.running) {
      return;
    }
    try {
      sensor.on(sensor.SensorId.ACCELEROMETER, this.onAccel, { interval: 20000000 }); // 20ms: 50Hz
      this.running = true;
      this.error = '';
    } catch (err) {
      this.error = `${err}`;
    }
  }

  private stop(): void {
    if (!this.running) {
      return;
    }
    try {
      sensor.off(sensor.SensorId.ACCELEROMETER, this.onAccel);
    } catch (err) {
      // already off
    }
    this.running = false;
  }

  private line(): string {
    if (this.error.length > 0) {
      return this.error;
    }
    const l = this.level;
    if (l === undefined) {
      return 'waiting for the accelerometer…';
    }
    const signed = (v: number): string => (v >= 0 ? '+' : '') + v.toFixed(1);
    return `pitch ${signed(l.pitch)}° · roll ${signed(l.roll)}° · ${l.rate.toFixed(0)} samples/s`;
  }

  build() {
    Row({ space: 12 }) {
      // The bubble: where the core says the phone is tipped. Clamped to the
      // circle, as a real level's is.
      Stack() {
        Circle()
          .width(22)
          .height(22)
          .fill($r('app.color.accent'))
          .translate({
            x: this.level === undefined ? 0 : Math.max(-20, Math.min(20, this.level.roll)),
            y: this.level === undefined ? 0 : Math.max(-20, Math.min(20, -this.level.pitch)),
          })
      }
      .width(64)
      .height(64)
      .borderRadius(32)
      .backgroundColor($r('app.color.surface_dim'))

      Text(this.line())
        .fontSize(11)
        .fontColor($r('app.color.text_secondary'))
        .layoutWeight(1)
        .maxLines(2)
    }
    .width('100%')
  }
}
