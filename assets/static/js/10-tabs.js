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

  setUpMenu();

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
 * The menu, below the breakpoint.
 *
 * The list is the same markup as the wide strip; only its presentation differs.
 * That means there is one set of links to keep correct, and the no-scripting
 * case is simply the list rendering as it is.
 */
function setUpMenu() {
  var button = document.getElementById("menu-btn");
  var list = document.getElementById("tabs-list");
  var tabs = list && list.closest(".tabs");
  if (!button || !tabs) return;

  function setOpen(open) {
    button.setAttribute("aria-expanded", open ? "true" : "false");
    if (open) {
      tabs.setAttribute("data-open", "");
    } else {
      tabs.removeAttribute("data-open");
    }
  }

  button.addEventListener("click", function () {
    setOpen(button.getAttribute("aria-expanded") !== "true");
  });

  document.addEventListener("keydown", function (e) {
    if (e.key !== "Escape") return;
    if (button.getAttribute("aria-expanded") !== "true") return;
    setOpen(false);
    // Focus goes back where it came from, or it lands nowhere and the next Tab
    // starts from the top of the document.
    button.focus();
  });

  // Otherwise it is still open behind the next page on a slow connection.
  tabs.addEventListener("click", function (e) {
    if (e.target.closest("a")) setOpen(false);
  });
}

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
