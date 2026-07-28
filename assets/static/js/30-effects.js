/* The two signature animations. */

/*
 * The background stream on the front page.
 *
 * It is not decorative noise: it is a real brainfuck program, and what it
 * prints is "Itemize NTNU". Once the source has finished typing out, the
 * output is appended — the payoff for anyone who watches long enough or
 * recognises the language.
 *
 * The irregular delay imitates a person typing rather than a machine.
 */
var BRAINFUCK =
  "++++++++[->++++++++<]>+++++++++.<++++++[->++++++<]>+++++++.<+++[->---<]>" +
  "------.++++++++.----.<++++[->++++<]>+.<++++[->----<]>-----.<++++++++[->-" +
  "-------<]>-----.<++++++[->++++++<]>++++++++++.++++++.------.+++++++.<+++" +
  "+++++[->--------<]>--------.---.>";

function brainfuck(el) {
  if (itemize.prefersReducedMotion()) return;

  var buffer = "";
  var i = 0;

  (function tick() {
    if (buffer.length >= 3000) {
      revealOutput(el);
      return;
    }
    var n = 2 + Math.floor(Math.random() * 2);
    for (var k = 0; k < n; k++) {
      buffer += BRAINFUCK[i++ % BRAINFUCK.length];
    }
    // textContent, never innerHTML: the program is full of < and >.
    el.textContent = buffer;
    setTimeout(
      tick,
      (30 + Math.pow(Math.random(), 2) * 100 + Math.pow(Math.random(), 6) * 500) *
        0.3,
    );
  })();
}

function revealOutput(el) {
  var out = document.createElement("span");
  out.className = "bf-out";
  out.textContent = "\n\n» Itemize NTNU\n";
  el.appendChild(out);
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
  var bg = document.getElementById("bf");
  if (bg) brainfuck(bg);

  // One reveal per page, not one per heading. A page where every heading
  // scrambles itself on load is a costume; a single one is a signature.
  itemize.all("[data-glitch]").forEach(function (el, i) {
    glitchReveal(el, 250 + i * 180);
  });
});
