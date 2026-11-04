// The one-line todo input. It owns its own draft text — nothing outside needs to
// know what is half-typed — and hands the finished title up.
//
// The list itself stays with the page, which owns the todos: one owner per piece
// of state, and this is the piece nobody else writes.

@ComponentV2
export struct TodoComposer {
  @Local private draft: string = '';
  @Event onAdd: (title: string) => void = (title: string): void => {};

  private submit(): void {
    const title = this.draft.trim();
    if (title.length === 0) {
      return;
    }
    this.onAdd(title);
    this.draft = '';
  }

  build() {
    Row({ space: 8 }) {
      TextInput({ text: this.draft, placeholder: 'What needs doing?' })
        .layoutWeight(1)
        .onChange((v: string) => {
          this.draft = v;
        })
        .onSubmit(() => {
          this.submit();
        })
      Button('Add')
        .enabled(this.draft.trim().length > 0)
        .onClick(() => {
          this.submit();
        })
    }
    .width('100%')
  }
}
