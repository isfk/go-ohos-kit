// A photo, end to end: the platform takes it and decodes it, the core does the
// maths, the UI draws the result.
//
// The split is the point. HarmonyOS's decoder reads every format the camera
// writes (HEIF included) and can scale — that's not the core's business. The
// core gets plain RGBA and answers questions about light: how bright, where,
// and out of which colours.

import { cameraPicker, camera } from '@kit.CameraKit';
import { image } from '@kit.ImageKit';
import { fileIo as fs } from '@kit.CoreFileKit';
import { photoAccessHelper } from '@kit.MediaLibraryKit';
import { BusinessError } from '@kit.BasicServicesKit';
import { AnalyzeResult, FileInfo, gocore, ImageAnalysis } from '../../core/GoCore';

/** Photos are decoded to this on the wide side: 12 MP would only cost time. */
const ANALYSIS_WIDTH = 320;

@ComponentV2
export struct PhotoCard {
  @Local private file: FileInfo | undefined = undefined;
  @Local private analysis: ImageAnalysis | undefined = undefined;
  @Local private status: string = 'no photo yet';
  @Local private busy: boolean = false;

  private async fromGallery(): Promise<void> {
    try {
      const picker = new photoAccessHelper.PhotoViewPicker();
      const chosen = await picker.select({
        MIMEType: photoAccessHelper.PhotoViewMIMETypes.IMAGE_TYPE,
        maxSelectNumber: 1,
      });
      if (chosen.photoUris.length === 0) {
        return;
      }
      await this.analyze(chosen.photoUris[0]);
    } catch (err) {
      this.status = `${(err as BusinessError).message}`;
    }
  }

  /** The system's camera UI: it takes the picture, we get a URI back. */
  private async fromCamera(): Promise<void> {
    if (this.busy) {
      return;
    }
    this.busy = true;
    try {
      const res = await cameraPicker.pick(
        getContext(this), [cameraPicker.PickerMediaType.PHOTO],
        { cameraPosition: camera.CameraPosition.CAMERA_POSITION_BACK });
      if (res.resultCode !== 0 || res.resultUri.length === 0) {
        this.status = res.resultCode === 0 ? 'no photo taken' : `camera picker: ${res.resultCode}`;
        return;
      }
      await this.analyze(res.resultUri);
    } catch (err) {
      this.status = `${(err as BusinessError).message}`;
    } finally {
      this.busy = false;
    }
  }

  /** One photo: what the file is (the core reads it), then what it looks like. */
  private async analyze(uri: string): Promise<void> {
    this.status = 'reading…';
    const parts = uri.split('/');
    const name = parts.length > 0 ? parts[parts.length - 1] : 'photo';
    const file = fs.openSync(uri, fs.OpenMode.READ_ONLY);
    try {
      const inspected = gocore.inspect(file.fd, name);
      this.file = inspected.ok ? inspected.file : undefined;

      // The platform decodes and scales; the core never sees a JPEG.
      const source = image.createImageSource(file.fd);
      const info = await source.getImageInfo();
      const scale = Math.min(1, ANALYSIS_WIDTH / Math.max(info.size.width, info.size.height));
      const width = Math.max(1, Math.round(info.size.width * scale));
      const height = Math.max(1, Math.round(info.size.height * scale));
      const pixels = await source.createPixelMap({
        desiredSize: { width: width, height: height },
        desiredPixelFormat: image.PixelMapFormat.RGBA_8888,
      });
      const buffer = new ArrayBuffer(width * height * 4);
      await pixels.readPixelsToBuffer(buffer);
      pixels.release();
      source.release();

      const result: AnalyzeResult = gocore.analyze(buffer, width, height);
      if (!result.ok || result.image === undefined) {
        this.analysis = undefined;
        this.status = result.error ?? 'the core could not read the pixels';
        return;
      }
      this.analysis = result.image;
      const a = result.image;
      const kb = (buffer.byteLength / 1024).toFixed(0);
      const ms = (a.micros / 1000).toFixed(1);
      this.status = `${a.width}×${a.height} · ${kb} KB RGBA · ${a.meanLuma}% bright · ${ms} ms in Go`;
    } catch (err) {
      this.analysis = undefined;
      this.status = `decode: ${(err as BusinessError).message}`;
    } finally {
      fs.closeSync(file);   // the descriptor was only borrowed
    }
  }

  private fileLine(): string {
    const f = this.file;
    if (f === undefined) {
      return '';
    }
    const mb = (f.bytes / (1024 * 1024)).toFixed(1);
    return `${f.kind} · ${mb} MB · ${f.sha256.substring(0, 12)}…`;
  }

  /** 8×8 squares: where the light falls, not one average for the whole frame. */
  @Builder
  lightGrid(a: ImageAnalysis) {
    Column({ space: 1 }) {
      ForEach([0, 1, 2, 3, 4, 5, 6, 7], (row: number) => {
        Row({ space: 1 }) {
          ForEach([0, 1, 2, 3, 4, 5, 6, 7], (col: number) => {
            Row()
              .width(11)
              .height(11)
              .borderRadius(2)
              .backgroundColor(`rgba(0, 0, 0, ${(a.grid[row * 8 + col] / 100).toFixed(2)})`)
              .borderWidth(0.5)
              .borderColor($r('app.color.hairline'))
          }, (col: number) => `${row}-${col}`)
        }
      }, (row: number) => `${row}`)
    }
  }

  /** The same numbers as bars: darkest on the left, brightest on the right. */
  @Builder
  histogram(a: ImageAnalysis) {
    Row({ space: 3 }) {
      ForEach(a.histo, (pct: number, i: number) => {
        Column() {
          Blank()
          Row()
            .width(14)
            .height(Math.max(1, pct) * 0.6)   // 100% → 60pt tall
            .borderRadius(2)
            .backgroundColor(i < 3 ? '#33000000' : i < 6 ? '#66000000' : '#CC000000')
        }
        .height(60)
        .justifyContent(FlexAlign.End)
      }, (pct: number, i: number) => `${i}`)
    }
    .height(60)
  }

  build() {
    Column({ space: 8 }) {
      Row({ space: 12 }) {
        Button('Photo')
          .fontSize(14)
          .backgroundColor($r('app.color.accent_tint'))
          .fontColor($r('app.color.accent'))
          .onClick(() => {
            this.fromGallery();
          })
        Button('Camera')
          .fontSize(14)
          .backgroundColor($r('app.color.accent_tint'))
          .fontColor($r('app.color.accent'))
          .enabled(!this.busy)
          .onClick(() => {
            this.fromCamera();
          })
        Text(this.status)
          .fontSize(11)
          .fontColor($r('app.color.text_secondary'))
          .layoutWeight(1)
          .maxLines(2)
      }
      .width('100%')

      if (this.analysis !== undefined) {
        Row({ space: 16 }) {
          this.lightGrid(this.analysis)
          Column({ space: 6 }) {
            this.histogram(this.analysis)
            Row({ space: 8 }) {
              ForEach(this.analysis.colors, (c: ColorShareLike) => {
                Row({ space: 4 }) {
                  Row()
                    .width(10)
                    .height(10)
                    .borderRadius(5)
                    .backgroundColor(c.hex)
                    .borderWidth(0.5)
                    .borderColor('#33000000')
                  Text(`${Math.round(c.share * 100)}%`)
                    .fontSize(10)
                    .fontColor($r('app.color.text_secondary'))
                }
              }, (c: ColorShareLike) => c.hex)
            }
            Text(this.fileLine())
              .fontSize(10)
              .fontColor($r('app.color.text_hint'))
              .maxLines(1)
              .textOverflow({ overflow: TextOverflow.Ellipsis })
          }
          .alignItems(HorizontalAlign.Start)
          .layoutWeight(1)
        }
        .width('100%')
        .alignItems(VerticalAlign.Top)
      }
    }
    .width('100%')
  }
}

/** ArkTS wants the shape spelled out where it's used in a lambda. */
interface ColorShareLike {
  hex: string;
  share: number;
}
