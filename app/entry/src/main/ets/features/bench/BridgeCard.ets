// What a call across the bridge costs when there is nothing in it — the floor
// under every synchronous "ArkTS asks the core something" on the UI thread.
// Three measurements, because three different things are in the way:
//
// - int back (remaining): NAPI call, cgo jump, C.int back. Pure overhead.
// - string back (todos): the same plus CString malloc/free, the copy into a JS
//   string, and JSON.parse. This is what a query result costs today.
// - 300 KB in (analyze): overhead plus the buffer copy. The core reports its own
//   maths time (micros), so total minus maths is the bridge's share.
//
// A few microseconds means a database query can be a synchronous call; a few
// hundred means it has to go through the event channel instead.

import { systemDateTime } from '@kit.BasicServicesKit';
import { gocore } from '../../core/GoCore';

@ComponentV2
export struct BridgeCard {
  @Local private result: string = '';

  /** Nanoseconds since boot: Date.now() is far too coarse for one call. */
  private nowNs(): number {
    return systemDateTime.getUptime(systemDateTime.TimeType.ACTIVE, true);
  }

  private run(): void {
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

    // 300 KB in: overhead plus the copy, with the core's clock to separate the
    // maths from the crossing.
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
    this.result = `int ${intNs.toFixed(0)}ns · json+parse ${us(strNs)}µs` +
      ` · ${kb.toFixed(0)}KB in ${us(pixNs)}µs (math ${us(mathNs)}µs → bridge ${us(pixNs - mathNs)}µs)`;
    // Same numbers in the log: a screenshot is evidence, text is easier to cite.
    console.info(`gokit-bench bridge: ${this.result}`);
  }

  build() {
    Row({ space: 12 }) {
      Button('Bridge µs')
        .fontSize(14)
        .backgroundColor($r('app.color.accent_tint'))
        .fontColor($r('app.color.accent'))
        .onClick(() => {
          this.run();
        })
      Text(this.result.length > 0 ? this.result : 'How expensive is a call?')
        .fontSize(11)
        .fontColor($r('app.color.text_secondary'))
        .layoutWeight(1)
        .maxLines(4)
    }
    .width('100%')
  }
}
