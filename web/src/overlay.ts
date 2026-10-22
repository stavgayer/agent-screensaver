/** Class on <html> while macOS shows its lock screen over the saver. */
export const LOCK_UI_CLASS = "lock-ui";

/**
 * Hides every component (and the service line) while the lock screen shows,
 * so its clock and login UI are not doubled by ours. The background keeps
 * running. Anything but `true` shows the components: a bad call never leaves
 * the screen empty.
 */
export function setLockUI(root: Pick<Element, "classList">, shown: unknown): void {
  root.classList.toggle(LOCK_UI_CLASS, shown === true);
}
