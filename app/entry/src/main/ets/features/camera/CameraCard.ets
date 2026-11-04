// The camera, the way a HarmonyOS app does it: the platform's preview into an
// XComponent's surface, a permission asked once, and everything released when
// the page goes away.
//
// What this proves for the Go core + ArkUI split: a runtime permission, a
// native surface handed to a system service, and a session that must be torn
// down on a lifecycle callback — the three things a "Kit" really costs.

import { camera } from '@kit.CameraKit';
import { abilityAccessCtrl, common, Permissions } from '@kit.AbilityKit';
import { BusinessError } from '@kit.BasicServicesKit';
import { AppLife, appLife } from '../../state/AppLife';

@ComponentV2
export struct CameraCard {
  @Local private status: string = 'camera off';
  @Local private running: boolean = false;
  private controller: XComponentController = new XComponentController();
  private session: camera.PhotoSession | undefined = undefined;
  private surfaceId: string = '';
  private wanted: boolean = false;
  /** The preview's own aspect: a fixed height would squash the frames. */
  @Local private aspect: number = 4 / 3;

  // The page says when it's gone, and the ability says when the app goes to
  // the background: a camera left open holds the device and eats the battery,
  // so neither is optional. (aboutToDisappear alone misses the home button —
  // the page is still alive, just invisible.)
  @Local private life: AppLife = appLife();

  @Monitor('life.foreground')
  onForegroundChanged(): void {
    if (!this.life.foreground) {
      this.onBackgrounded();
    }
  }

  aboutToDisappear(): void {
    this.stop();
  }

  private onBackgrounded(): void {
    if (this.session !== undefined) {
      this.stop();
      this.status = 'camera off (backgrounded)';
    }
  }

  private async start(): Promise<void> {
    this.wanted = true;
    if (this.surfaceId.length === 0) {
      this.status = 'waiting for the surface…';
      return;
    }
    await this.askPermission();
  }

  private async askPermission(): Promise<void> {
    const permissions: Permissions[] = ['ohos.permission.CAMERA'];
    const ctx = getContext(this) as common.UIAbilityContext;
    try {
      const at = abilityAccessCtrl.createAtManager();
      const result = await at.requestPermissionsFromUser(ctx, permissions);
      if (result.authResults.length === 0 || result.authResults[0] !== 0) {
        this.status = 'camera permission denied';
        this.wanted = false;
        return;
      }
    } catch (err) {
      this.status = `permission: ${(err as BusinessError).message}`;
      this.wanted = false;
      return;
    }
    this.openCamera(ctx);
  }

  /** Opens the camera and puts its preview on the XComponent's surface. */
  private openCamera(ctx: common.UIAbilityContext): void {
    try {
      const manager: camera.CameraManager = camera.getCameraManager(ctx);
      const cameras: camera.CameraDevice[] = manager.getSupportedCameras();
      const back: camera.CameraDevice | undefined = cameras.find(
        (c: camera.CameraDevice) => c.cameraPosition === camera.CameraPosition.CAMERA_POSITION_BACK);
      if (back === undefined) {
        this.status = 'no back camera';
        return;
      }
      const capability: camera.CameraOutputCapability =
        manager.getSupportedOutputCapability(back, camera.SceneMode.NORMAL_PHOTO);
      if (capability.previewProfiles.length === 0) {
        this.status = 'no preview profile';
        return;
      }
      const input: camera.CameraInput = manager.createCameraInput(back);
      // A preview-sized profile: the camera's largest is wasted on a panel
      // this small and costs power, so take the biggest at or below 1280.
      let profile: camera.Profile = capability.previewProfiles[0];
      for (const p of capability.previewProfiles) {
        if (p.size.width <= 1280 && p.size.width > profile.size.width) {
          profile = p;
        }
      }
      this.aspect = profile.size.width / profile.size.height;
      const preview: camera.PreviewOutput = manager.createPreviewOutput(profile, this.surfaceId);
      const session: camera.PhotoSession = manager.createSession<camera.PhotoSession>(camera.SceneMode.NORMAL_PHOTO);

      input.open().then(() => {
        session.beginConfig();
        session.addInput(input);
        session.addOutput(preview);
        session.commitConfig();
        // start() isn't a promise in every version of this API (a .then on a
        // void silently threw, and the UI never learned it was live): take the
        // session as running, and let a real failure say otherwise.
        session.start();
        this.session = session;
        this.running = true;
        this.status = `${profile.size.width}×${profile.size.height} preview`;
      }).catch((err: BusinessError) => {
        this.status = `open: ${err.message}`;
      });
    } catch (err) {
      this.status = `${(err as BusinessError).message}`;
    }
  }

  private stop(): void {
    this.wanted = false;
    const session = this.session;
    this.session = undefined;
    this.running = false;
    if (session === undefined) {
      return;
    }
    try {
      session.stop();
      session.release();
      this.status = 'camera off';
    } catch (err) {
      this.status = `stop: ${(err as BusinessError).message}`;
    }
  }

  build() {
    Column({ space: 8 }) {
      Row({ space: 12 }) {
        Button(this.running ? 'Stop camera' : 'Start camera')
          .fontSize(14)
          .backgroundColor($r('app.color.accent_tint'))
          .fontColor($r('app.color.accent'))
          .onClick(() => {
            if (this.running) {
              this.stop();
            } else {
              this.start();
            }
          })
        Text(this.status)
          .fontSize(11)
          .fontColor($r('app.color.text_secondary'))
          .layoutWeight(1)
          .maxLines(2)
      }
      .width('100%')

      // The panel the platform draws into: its surface is what the camera
      // service writes frames to, and the UI never sees them. It keeps its
      // size when idle — a zero-height XComponent has no surface to give — and
      // says so on top of it.
      Stack() {
        XComponent({
          id: 'cameraPreview',
          type: XComponentType.SURFACE,
          controller: this.controller,
        })
          .width('100%')
          .height('100%')
          .onLoad(() => {
            this.surfaceId = this.controller.getXComponentSurfaceId();
            if (this.wanted && !this.running) {
              this.askPermission();
            }
          })
        // Never removed: taking a sibling out of a Stack rebuilds the
        // XComponent, and a rebuilt one has a new surface — the running
        // session would keep drawing into the old one (a black panel).
        Text('no preview')
          .fontSize(12)
          .fontColor($r('app.color.text_hint'))
          .opacity(this.running ? 0 : 1)
      }
      .width('100%')
      .aspectRatio(this.aspect)
      .borderRadius(12)
      .backgroundColor($r('app.color.surface_dim'))
      .clip(true)
    }
    .width('100%')
  }
}
