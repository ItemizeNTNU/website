/* The two signature animations. */

/*
 * The front page sequence.
 *
 * The program source is already in the markup, so with scripting off the hero
 * reads as a finished terminal session. With scripting, it is cleared and
 * typed back in, then the output — the wordmark — fades in beneath it.
 *
 * The irregular delay imitates a person typing rather than a machine.
 */
function runHeroSequence(src, out) {
  var program = src.textContent.trim();
  if (!program) return;

  if (itemize.prefersReducedMotion()) return; // already complete in the markup

  // Hide the output until the program has "run", and start from an empty
  // buffer. Both are undone by the sequence itself, so a script that fails
  // partway leaves the page readable rather than blank.
  if (out) out.removeAttribute("data-shown");
  src.textContent = "";

  var typed = 0;
  (function tick() {
    if (typed >= program.length) {
      if (out) out.setAttribute("data-shown", "");
      return;
    }
    typed += 2 + Math.floor(Math.random() * 3);
    // textContent, never innerHTML: the program is full of < and >.
    src.textContent = program.slice(0, typed);
    setTimeout(
      tick,
      12 + Math.pow(Math.random(), 2) * 40 + Math.pow(Math.random(), 6) * 160,
    );
  })();
}

/*
 * Runs fn once the arrival animation has finished, or straight away if there is
 * none — reduced motion, no command on this page, or the script never ran.
 */
function whenArrived(fn) {
  var pane = document.querySelector(".shell__pane");
  if (!pane || !pane.hasAttribute("data-arriving")) {
    fn();
    return;
  }
  var observer = new MutationObserver(function () {
    if (!pane.hasAttribute("data-arriving")) {
      observer.disconnect();
      fn();
    }
  });
  observer.observe(pane, { attributes: true, attributeFilter: ["data-arriving"] });
}

/*
 * Scrambles an element's text, then locks it in from the left.
 *
 * The real text is always present in the markup and the accessible name is set
 * on the anchor, so a screen reader never reads a half-resolved string and the
 * content is intact with scripting disabled.
 */
var GLITCH_CHARS = "!@#$%^&*()_+-=[]{}|;:,.<>?/~`0123456789abcdef";

function randomChar() {
  return GLITCH_CHARS[Math.floor(Math.random() * GLITCH_CHARS.length)];
}

function scramble(text, revealed) {
  var out = "";
  for (var i = 0; i < text.length; i++) {
    out += i < revealed ? text[i] : randomChar();
  }
  return out;
}

function glitchReveal(el, startDelay) {
  var text = el.textContent.trim();
  if (!text || itemize.prefersReducedMotion()) return;

  el.textContent = scramble(text, 0);
  var scrambling = setInterval(function () {
    el.textContent = scramble(text, 0);
  }, 80);

  setTimeout(function () {
    clearInterval(scrambling);
    var revealed = 0;
    var revealing = setInterval(function () {
      el.textContent = scramble(text, revealed);
      if (++revealed > text.length) {
        clearInterval(revealing);
        el.textContent = text;
      }
    }, 35);
  }, startDelay);
}

/*
 * Entry point last, not first.
 *
 * app.js is deferred, so by the time it runs the document is already parsed
 * and itemize.ready() invokes its callback synchronously. A `var` further down
 * the file would still be undefined at that point — hoisted, but unassigned.
 * Keeping the call at the bottom means everything it touches is initialised.
 */
itemize.ready(function () {
  var src = document.getElementById("bf");
  if (src) {
    // The pane fades its content in once the page's own command has typed. On
    // the front page that content is this sequence, so starting it immediately
    // would run the program behind a fade and finish before anyone saw it.
    // 60-session.js clears the attribute when it is done.
    whenArrived(function () {
      runHeroSequence(src, document.getElementById("hero-out"));
    });
  }

  // One reveal per page, not one per heading. A page where every heading
  // scrambles itself on load is a costume; a single one is a signature.
  itemize.all("[data-glitch]").forEach(function (el, i) {
    glitchReveal(el, 250 + i * 180);
  });
});
