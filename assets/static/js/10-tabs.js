/* The window's tabs. */

itemize.ready(function () {
  var tabs = itemize.all("[data-tab]");
  if (!tabs.length) return;

  // On a narrow screen the strip scrolls, so the current tab may start out of
  // sight. "nearest" rather than "center" so a tab already visible does not
  // jolt the strip sideways for no reason.
  var active = document.querySelector('[data-tab][aria-current="page"]');
  if (active && active.scrollIntoView) {
    active.scrollIntoView({ block: "nearest", inline: "nearest" });
  }

  document.addEventListener("keydown", function (e) {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    if (!shortcutAllowed(e)) return;

    var index = tabs.indexOf(active);
    if (index < 0) index = 0;

    // Wrapping is what a tabbed terminal does, and it means the shortcut never
    // silently stops working at the ends.
    var next =
      e.key === "ArrowRight"
        ? (index + 1) % tabs.length
        : (index - 1 + tabs.length) % tabs.length;

    e.preventDefault();
    window.location.href = tabs[next].href;
  });
});

/*
 * Whether a bare arrow key should be treated as a shortcut.
 *
 * Every one of these guards is load-bearing. Without the first, arrow keys stop
 * working inside the console and inside every form on the site — you could not
 * move the caret in a field. Without the second, browser and window-manager
 * shortcuts get swallowed. Without the third, a confirmation dialog could be
 * navigated out from under.
 */
function shortcutAllowed(e) {
  if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return false;

  var el = document.activeElement;
  if (el) {
    var tag = el.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return false;
    if (el.isContentEditable) return false;
  }

  if (document.querySelector("dialog[open]")) return false;

  return true;
}
