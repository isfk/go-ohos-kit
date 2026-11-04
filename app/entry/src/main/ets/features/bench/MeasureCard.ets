// The measurement card for the wire format: the same 2000 rows twice, once as
// JSON (bytes → string → objects) and once in the compact form (bytes →
// objects). This is what the binary path is for, and what README's table says.

import { gocore, Row } from '../../core/GoCore';

@ComponentV2
export struct MeasureCard {
  @Local private result: string = '';

  private run(): void {
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
    this.result = `${gocore.rowsCount()} rows · ${page}: JSON ${jsonMs.toFixed(1)}ms/${(jsonBytes / kb).toFixed(0)}KB` +
      ` · binary ${binMs.toFixed(1)}ms/${(binBytes / kb).toFixed(0)}KB · wire only ${wireMs.toFixed(1)}ms` +
      ` · e.g. ${first}`;
    console.info(`gokit-bench wire: ${this.result}`);
  }

  build() {
    Row({ space: 12 }) {
      Button('Measure 20k')
        .fontSize(14)
        .backgroundColor($r('app.color.accent_tint'))
        .fontColor($r('app.color.accent'))
        .onClick(() => {
          this.run();
        })
      Text(this.result.length > 0 ? this.result : 'JSON or the compact form?')
        .fontSize(11)
        .fontColor($r('app.color.text_secondary'))
        .layoutWeight(1)
        .maxLines(3)
    }
    .width('100%')
  }
}
