// The core's goroutine pushing a line a second: the bridge, both ways. Its own
// component so a tick re-renders one Text instead of the whole page.

import { Event } from '../../core/GoCore';
import { onCoreEvent } from '../../core/events';

@ComponentV2
export struct Heartbeat {
  @Local private line: string = '';
  private off: () => void = (): void => {};

  aboutToAppear(): void {
    this.off = onCoreEvent('text', (e: Event): void => {
      this.line = e.text ?? '';
    });
  }

  aboutToDisappear(): void {
    this.off();
  }

  build() {
    Text(this.line)
      .fontSize(13)
      .fontColor($r('app.color.text_secondary'))
  }
}
