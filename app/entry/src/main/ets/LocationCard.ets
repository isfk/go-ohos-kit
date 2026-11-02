// The third kind of platform capability: a stream with a subscription.
//
// Location Kit measures where you are (that's the platform's job: GNSS, wifi,
// cells). The core turns successive fixes into what a person actually asks:
// how far have I walked, in which direction, how fast.

import { abilityAccessCtrl, common, Permissions } from '@kit.AbilityKit';
import { geoLocationManager } from '@kit.LocationKit';
import { backgroundTaskManager } from '@kit.BackgroundTasksKit';
import { wantAgent } from '@kit.AbilityKit';
import { BusinessError } from '@kit.BasicServicesKit';
import { GeoPoint, gocore } from './GoCore';
import { AppLife, appLife } from './AppState';

@ComponentV2
export struct LocationCard {
  @Local private status: string = 'not tracking';
  @Local private point: GeoPoint | undefined = undefined;
  // @Local, or the button's label won't follow: a plain field changes the
  // value and leaves the UI showing the old one. (V2 keeps its own set of
  // decorators — @State belongs to the V1 world and can't be mixed in.)
  @Local private watching: boolean = false;
  /**
   * A continuous task is running: the system keeps this process alive in the
   * background (and shows a notification saying so).
   */
  @Local private bgTask: boolean = false;
  private listener: ((loc: geoLocationManager.Location) => void) | undefined = undefined;

  // A GPS subscription left running in the background is a battery bug: stop it
  // when the app leaves the screen, exactly like the camera.
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
    if (!this.watching) {
      return;
    }
    if (this.bgTask) {
      // The continuous task is exactly what earns the right to keep the GPS
      // subscription alive here.
      this.status = 'tracking in the background';
      return;
    }
    this.stop();
    this.status = 'not tracking (backgrounded)';
  }

  /** Coarse and precise together: asking for LOCATION alone is refused. */
  private async askPermission(): Promise<boolean> {
    const permissions: Permissions[] = [
      'ohos.permission.APPROXIMATELY_LOCATION',
      'ohos.permission.LOCATION',
    ];
    try {
      const at = abilityAccessCtrl.createAtManager();
      const result = await at.requestPermissionsFromUser(
        getContext(this) as common.UIAbilityContext, permissions);
      if (result.authResults.length < 2 || result.authResults[0] !== 0) {
        this.status = 'location permission denied';
        return false;
      }
      return true;
    } catch (err) {
      this.status = `permission: ${(err as BusinessError).message}`;
      return false;
    }
  }

  private async start(): Promise<void> {
    if (!await this.askPermission()) {
      return;
    }
    const request: geoLocationManager.ContinuousLocationRequest = {
      interval: 1,
      locationScenario: geoLocationManager.PowerConsumptionScenario.HIGH_POWER_CONSUMPTION,
    };
    const listener = (loc: geoLocationManager.Location): void => {
      // One line of ArkTS per fix: the core does the rest.
      const result = gocore.location(loc.latitude, loc.longitude, loc.accuracy);
      if (!result.ok || result.point === undefined) {
        this.status = result.error ?? 'the core could not use that fix';
        return;
      }
      this.point = result.point;
      const p = result.point;
      this.status = `${p.lat.toFixed(5)}, ${p.lon.toFixed(5)} · ±${p.accuracy.toFixed(0)} m`;
    };
    this.listener = listener;
    gocore.locationReset();
    geoLocationManager.on('locationChange', request, listener);
    this.watching = true;
    this.status = 'waiting for a fix… (indoors, this can take a while)';
  }

  /**
   * A continuous task: the app asks the system for permission to stay alive
   * off-screen, and gets a notification in return (the user can see, and stop,
   * what is running). Declared in module.json5 as backgroundModes: ["location"].
   */
  private async startBackground(): Promise<void> {
    const ctx = getContext(this) as common.UIAbilityContext;
    try {
      const info: wantAgent.WantAgentInfo = {
        wants: [{
          bundleName: ctx.abilityInfo.bundleName,
          abilityName: ctx.abilityInfo.name,
        }],
        actionType: wantAgent.OperationType.START_ABILITY,
        requestCode: 0,
        wantAgentFlags: [wantAgent.WantAgentFlags.UPDATE_PRESENT_FLAG],
      };
      const agent = await wantAgent.getWantAgent(info);
      await backgroundTaskManager.startBackgroundRunning(ctx, ['location'], agent);
      this.bgTask = true;
      this.status = 'tracking · continuous task on';
    } catch (err) {
      this.status = `background: ${(err as BusinessError).message}`;
    }
  }

  private async stopBackground(): Promise<void> {
    const ctx = getContext(this) as common.UIAbilityContext;
    try {
      await backgroundTaskManager.stopBackgroundRunning(ctx);
    } catch (err) {
      // Stopping something that already ended is not an error worth the UI.
    }
    this.bgTask = false;
  }

  private stop(): void {
    if (!this.watching) {
      return;
    }
    this.watching = false;
    if (this.listener !== undefined) {
      geoLocationManager.off('locationChange', this.listener);
      this.listener = undefined;
    }
    if (this.bgTask) {
      this.stopBackground();
    }
    gocore.locationReset();
    this.point = undefined;   // the track is gone: don't leave its numbers behind
    this.status = 'not tracking';
  }

  private trackLine(): string {
    const p = this.point;
    if (p === undefined) {
      return '';
    }
    // Standing still has no direction: a 0° reading there is an artefact of
    // atan2(0, 0), not information.
    const bearing = p.moved > 1 ? ` · bearing ${p.bearing.toFixed(0)}°` : '';
    return `${p.moved.toFixed(0)} m walked · ${p.speed.toFixed(1)} m/s · ${p.fixes} fixes` +
      ` · ${p.seconds.toFixed(0)}s${bearing}`;
  }

  build() {
    Column({ space: 8 }) {
      Row({ space: 12 }) {
        Button(this.watching ? 'Stop location' : 'Locate')
          .fontSize(14)
          .backgroundColor('#1F0A59F7')
          .fontColor('#0A59F7')
          .onClick(() => {
            if (this.watching) {
              this.stop();
            } else {
              this.start();
            }
          })
        if (this.watching) {
          Button(this.bgTask ? 'Stop background' : 'Background')
            .fontSize(14)
            .backgroundColor('#1F0A59F7')
            .fontColor('#0A59F7')
            .onClick(() => {
              if (this.bgTask) {
                this.stopBackground();
                this.status = 'tracking (foreground only)';
              } else {
                this.startBackground();
              }
            })
        }
        Text(this.status)
          .fontSize(11)
          .fontColor('#99000000')
          .layoutWeight(1)
          .maxLines(2)
      }
      .width('100%')

      if (this.point !== undefined) {
        Text(this.trackLine())
          .fontSize(11)
          .fontColor('#66000000')
          .width('100%')
      }
    }
    .width('100%')
  }
}
