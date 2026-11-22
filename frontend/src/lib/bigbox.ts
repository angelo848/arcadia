import {
  WindowFullscreen,
  WindowIsFullscreen,
  WindowMaximise,
  WindowUnfullscreen,
  WindowUnmaximise,
} from '../../wailsjs/runtime/runtime'

// Big Box window helpers: full screen and foreground (BBX-001, CON-022).

// enterBigBox puts the window in full screen (BIG-001). It asks the window first, so a window that is
// already full screen is not toggled back. Outside the desktop shell (tests, browser) it does nothing.
export async function enterBigBox(): Promise<void> {
  try {
    if (!(await WindowIsFullscreen())) {
      WindowFullscreen()
    }
  } catch {
    // no window runtime: nothing to change
  }
}

// exitBigBox leaves full screen (BIG-001), only when the window is in full screen. Leaving it restores the
// maximised window over the whole monitor, under the taskbar; maximising again fits the work area.
export async function exitBigBox(): Promise<void> {
  try {
    if (await WindowIsFullscreen()) {
      WindowUnfullscreen()
      setTimeout(() => {
        try {
          WindowUnmaximise()
          WindowMaximise()
        } catch {
          // no window runtime: nothing to change
        }
      }, 150)
    }
  } catch {
    // no window runtime: nothing to change
  }
}

// takeBigBoxRequest consumes the pending Big Box request of the agent [ATB-012]. The binding is read at call
// time; only a literal true counts, and a failure or a missing binding means no request.
export async function takeBigBoxRequest(): Promise<boolean> {
  try {
    const app = (window as unknown as { go?: { main?: { App?: { TakeBigBoxRequest?: () => Promise<unknown> } } } }).go?.main?.App
    return (await app?.TakeBigBoxRequest?.()) === true
  } catch {
    return false
  }
}

// bringToFront asks the shell to put the Arcadia window in the foreground [CON-022]. The binding is read
// at call time; a missing binding or a failure is ignored (the Windows refusal is not an error).
export async function bringToFront(): Promise<void> {
  try {
    const app = (window as unknown as { go?: { main?: { App?: { BringToFront?: () => Promise<unknown> } } } }).go?.main?.App
    await app?.BringToFront?.()
  } catch {
    // ignored on purpose
  }
}
