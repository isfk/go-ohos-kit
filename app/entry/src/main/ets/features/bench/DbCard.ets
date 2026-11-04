// The fourth comparison, and the one this file exists for: the same 20k rows
// written by two databases that are both SQLite, from two sides of the bridge.
//
//   Go side   — the core's own file (modernc.org/sqlite, pure Go, no cgo)
//   RDB side  — the platform's relationalStore, which owns the file it manages
//
// They are deliberately two files, not two table names in one: one file has
// one writer. Two handles on the same file is SQLITE_BUSY, and it shows up
// only when it is inconvenient.
//
// Both numbers include building the rows (Go builds titles inside its call,
// the platform's writer builds ValuesBuckets here), because that is what the
// app actually pays. The Go side also reports the store's own clock, so the
// same call says what the bridge cost without a second experiment.

import { relationalStore } from '@kit.ArkData';
import { systemDateTime } from '@kit.BasicServicesKit';
import { gocore } from '../../core/GoCore';

const SEED = 20000; // what ../bench/sqlite calls a bulk import
const PAGE = 2000; // one screenful, the size the README quotes
const CHUNK = 2000; // batchInsert takes an array; 2k objects at a time

@ComponentV2
export struct DbCard {
  @Local private status: string = 'not opened';
  @Local private goSeedLine: string = '';
  @Local private goPageLine: string = '';
  @Local private rdbSeedLine: string = '';
  @Local private rdbPageLine: string = '';
  private rdb: relationalStore.RdbStore | undefined = undefined;

  aboutToAppear(): void {
    const opened = gocore.dbOpen(getContext(this).filesDir);
    this.status = opened.ok ? 'go db ready' : `go db: ${opened.error ?? 'open failed'}`;
  }

  /** Nanoseconds since boot: Date.now() can't see a single call. */
  private nowNs(): number {
    return systemDateTime.getUptime(systemDateTime.TimeType.ACTIVE, true);
  }

  private note(line: string): void {
    console.info(`gokit-bench db: ${line}`);
  }

  /** One bulk import, one transaction — the shape that ../bench/sqlite says matters most. */
  private goSeed(): void {
    const start = this.nowNs();
    const res = gocore.dbSeed(SEED);
    const total = (this.nowNs() - start) / 1e6;
    if (!res.ok) {
      this.goSeedLine = res.error ?? 'seed failed';
      return;
    }
    this.goSeedLine = `${total.toFixed(1)} ms · store ${(res.micros / 1000).toFixed(1)} ms · ${res.rows} rows`;
    this.note(`go seed ${SEED}: ${this.goSeedLine}`);
  }

  private goPage(): void {
    const start = this.nowNs();
    const res = gocore.dbPage(0, PAGE);
    const total = (this.nowNs() - start) / 1e6;
    if (!res.ok) {
      this.goPageLine = res.error ?? 'page failed';
      return;
    }
    this.goPageLine = `${total.toFixed(1)} ms · store ${(res.micros / 1000).toFixed(2)} ms · ${res.count} rows`;
    this.note(`go page ${PAGE}: ${this.goPageLine}`);
  }

  /** The platform's store, opened once and reused. */
  private async platform(): Promise<relationalStore.RdbStore> {
    const open = this.rdb;
    if (open !== undefined) {
      return open;
    }
    const config: relationalStore.StoreConfig = {
      name: 'bench.db',
      securityLevel: relationalStore.SecurityLevel.S1,
    };
    const store = await relationalStore.getRdbStore(getContext(this), config);
    await store.executeSql('CREATE TABLE IF NOT EXISTS bench (id INTEGER PRIMARY KEY, title TEXT NOT NULL, done INTEGER NOT NULL)');
    this.rdb = store;
    return store;
  }

  private async rdbSeed(): Promise<void> {
    const store = await this.platform();
    const start = this.nowNs();
    await store.executeSql('DELETE FROM bench');
    for (let off = 0; off < SEED; off += CHUNK) {
      const end = Math.min(off + CHUNK, SEED);
      const values: relationalStore.ValuesBucket[] = [];
      for (let i = off; i < end; i++) {
        const row: relationalStore.ValuesBucket = {
          'id': i,
          'title': `row ${i}`,
          'done': i % 2 === 0 ? 1 : 0,
        };
        values.push(row);
      }
      await store.batchInsert('bench', values);
    }
    const total = (this.nowNs() - start) / 1e6;
    this.rdbSeedLine = `${total.toFixed(1)} ms · ${SEED} rows in ${SEED / CHUNK} batches`;
    this.note(`rdb seed ${SEED}: ${this.rdbSeedLine}`);
  }

  private async rdbPage(): Promise<void> {
    const store = await this.platform();
    const start = this.nowNs();
    const set = await store.querySql('SELECT id, title, done FROM bench ORDER BY id LIMIT ? OFFSET ?', [PAGE, 0]);
    let n = 0;
    while (set.goToNextRow()) {
      set.getLong(0);
      set.getString(1);
      set.getLong(2);
      n++;
    }
    set.close();
    const total = (this.nowNs() - start) / 1e6;
    this.rdbPageLine = `${total.toFixed(1)} ms · ${n} rows`;
    this.note(`rdb page ${PAGE}: ${this.rdbPageLine}`);
  }

  private async resetBoth(): Promise<void> {
    gocore.dbReset();
    const store = await this.platform();
    await store.executeSql('DELETE FROM bench');
    this.goSeedLine = '';
    this.goPageLine = '';
    this.rdbSeedLine = '';
    this.rdbPageLine = '';
    this.status = 'both empty';
  }

  build() {
    Column({ space: 8 }) {
      Row({ space: 8 }) {
        Text('SQLite')
          .fontSize(13)
          .fontColor($r('app.color.text_secondary'))
        Blank()
        Text(this.status)
          .fontSize(11)
          .fontColor($r('app.color.text_secondary'))
      }
      .width('100%')

      // Go's own file, through the bridge: one transaction, one statement.
      Row({ space: 8 }) {
        Button('Go · seed 20k')
          .fontSize(12)
          .height(30)
          .backgroundColor($r('app.color.accent_tint'))
          .fontColor($r('app.color.accent'))
          .onClick(() => {
            this.goSeed();
          })
        Button('Go · page 2k')
          .fontSize(12)
          .height(30)
          .backgroundColor($r('app.color.accent_tint'))
          .fontColor($r('app.color.accent'))
          .onClick(() => {
            this.goPage();
          })
        Text(this.goSeedLine.length > 0 ? this.goSeedLine : this.goPageLine)
          .fontSize(11)
          .fontColor($r('app.color.text_secondary'))
          .layoutWeight(1)
          .maxLines(2)
      }
      .width('100%')

      // The platform's file, the same rows, from the other side.
      Row({ space: 8 }) {
        Button('RDB · seed 20k')
          .fontSize(12)
          .height(30)
          .backgroundColor($r('app.color.accent_tint'))
          .fontColor($r('app.color.accent'))
          .onClick(() => {
            this.rdbSeed();
          })
        Button('RDB · page 2k')
          .fontSize(12)
          .height(30)
          .backgroundColor($r('app.color.accent_tint'))
          .fontColor($r('app.color.accent'))
          .onClick(() => {
            this.rdbPage();
          })
        Text(this.rdbSeedLine.length > 0 ? this.rdbSeedLine : this.rdbPageLine)
          .fontSize(11)
          .fontColor($r('app.color.text_secondary'))
          .layoutWeight(1)
          .maxLines(2)
      }
      .width('100%')

      Row({ space: 8 }) {
        Button('Reset both')
          .fontSize(12)
          .height(30)
          .backgroundColor('#00000000')
          .fontColor($r('app.color.danger'))
          .onClick(() => {
            this.resetBoth();
          })
        Text(`${gocore.dbCount()} rows in the core's file`)
          .fontSize(11)
          .fontColor($r('app.color.text_secondary'))
          .layoutWeight(1)
      }
      .width('100%')
    }
    .width('100%')
  }
}
