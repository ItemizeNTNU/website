/*
 * Arrival: type the page's command, then let its output in.
 *
 * The point is that changing page reads as one session continuing rather than a
 * document being replaced. The command is already in the markup — this empties
 * the visible copy and types it back from the data attribute — so with
 * scripting off, under reduced motion, or if this never runs, the page is
 * simply complete.
 *
 * Numbered before the other behaviours on purpose. Callbacks registered with
 * itemize.ready run in file order, so this has to set data-arriving before
 * 30-effects.js checks for it — otherwise the front page's sequence starts
 * while the pane is still being faded out and finishes before anyone sees it.
 */

// About 300 ms for a typical command. Long enough to read as typing, short
// enough that four pages in a row never feels like waiting.
var TYPE_MS = 18;

itemize.ready(function () {
  var cmd = document.querySelector(".cmd[data-cmd]");
  var pane = document.querySelector(".shell__pane");
  if (!cmd || !pane) return;

  var text = cmd.getAttribute("data-cmd");
  if (!text || itemize.prefersReducedMotion()) return;

  var finished = false;
  var timer = null;

  function finish() {
    if (finished) return;
    finished = true;
    clearTimeout(timer);
    cmd.textContent = text;
    cmd.removeAttribute("data-typing");
    pane.removeAttribute("data-arriving");
    detach();
  }

  // Any input at all completes it immediately. A transition that cannot be
  // dismissed stops being an effect and becomes an obstacle on the third visit.
  // So does leaving the tab: this one gates the whole page's content, so it is
  // the last thing that should be left waiting on a throttled timer.
  var events = ["keydown", "pointerdown", "wheel", "touchstart"];
  var cancelHidden = function () {};

  function detach() {
    events.forEach(function (name) {
      window.removeEventListener(name, finish, { capture: true });
    });
    cancelHidden();
  }
  events.forEach(function (name) {
    window.addEventListener(name, finish, { capture: true, passive: true });
  });

  pane.setAttribute("data-arriving", "");
  cmd.setAttribute("data-typing", "");
  cmd.textContent = "";

  // Only now: onHidden calls back synchronously on a page that is already in
  // the background, and finishing before the attributes exist would set them
  // straight afterwards with nothing left running to take them off again.
  cancelHidden = itemize.onHidden(finish);
  if (finished) return;

  var i = 0;
  (function tick() {
    if (finished) return;
    if (i >= text.length) {
      finish();
      return;
    }
    cmd.textContent = text.slice(0, ++i);
    timer = setTimeout(tick, TYPE_MS);
  })();
});
