// The list, as plain rows rather than a List: it lives inside the page's Scroll,
// and a nested scrollable eats the gesture that would reach the cards below it
// (measured — a List at 220vp filled the whole viewport).
//
// No state of its own: the page owns the todos and this renders them. Toggles and
// deletes go out as events, the page decides what they mean.

import { Todo } from '../../core/GoCore';

@ComponentV2
export struct TodoList {
  @Param todos: Todo[] = [];
  @Event onToggle: (id: number) => void = (id: number): void => {};
  @Event onRemove: (id: number) => void = (id: number): void => {};

  /** Counted here because the list is the only thing showing it. */
  private remaining(): number {
    return this.todos.filter((t: Todo): boolean => !t.done).length;
  }

  build() {
    Column({ space: 12 }) {
      ForEach(this.todos, (t: Todo) => {
        Row({ space: 12 }) {
          Toggle({ type: ToggleType.Checkbox, isOn: t.done })
            .width(24)
            .height(24)
            .onChange(() => {
              this.onToggle(t.id);
            })
          Text(t.title)
            .fontSize(17)
            .layoutWeight(1)
            .decoration({
              type: t.done ? TextDecorationType.LineThrough : TextDecorationType.None,
              color: $r('app.color.text_secondary'),
            })
          Button('Delete')
            .fontSize(13)
            .height(30)
            .backgroundColor('#00000000')
            .fontColor($r('app.color.danger'))
            .onClick(() => {
              this.onRemove(t.id);
            })
        }
        .width('100%')
        .padding({ top: 8, bottom: 8 })
        .border({ width: { bottom: 0.5 }, color: $r('app.color.divider') })
      }, (t: Todo) => t.id.toString())

      Text(this.remaining() === 1 ? '1 thing left' : `${this.remaining()} things left`)
        .fontSize(13)
        .fontColor($r('app.color.text_secondary'))
        .width('100%')
    }
    .width('100%')
  }
}
