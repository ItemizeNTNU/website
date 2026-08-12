/*
 * Copy button for the calendar feed address.
 *
 * Strictly additive. The button is served with the hidden attribute and only
 * appears here, once the Clipboard API is known to exist — without it the
 * button would do nothing when pressed, and a control that fails silently is
 * worse than no control. When it never appears, the readonly input beside it
 * still holds the address, selectable and copyable by hand.
 */

itemize.ready(function () {
  if (!(navigator.clipboard && navigator.clipboard.writeText)) return;

  itemize.all("[data-copy]").forEach(function (btn) {
    var label = btn.textContent;
    var timer = null;

    btn.removeAttribute("hidden");
    btn.addEventListener("click", function () {
      navigator.clipboard.writeText(btn.getAttribute("data-copy")).then(
        function () {
          // Swap the label rather than raising a toast: the confirmation
          // belongs where the eyes already are, on the button just pressed.
          btn.textContent = "Kopiert!";
          clearTimeout(timer);
          timer = setTimeout(function () {
            btn.textContent = label;
          }, 2000);
        },
        function () {
          // The write can be refused (permissions policy, an insecure
          // context). Leaving the label alone would read as success, so say
          // what happened; the address stays copyable by hand.
          btn.textContent = "Kunne ikke kopiere";
          clearTimeout(timer);
          timer = setTimeout(function () {
            btn.textContent = label;
          }, 2000);
        },
      );
    });
  });
});
