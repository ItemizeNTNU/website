/*
 * Form enhancements.
 *
 * Strictly additive. Every form on this site submits and validates without
 * JavaScript, and every destructive action has a server-rendered confirmation
 * page; this only saves a round trip and prevents double submits.
 */

itemize.ready(function () {
  // Mark a form busy while it submits, so an impatient double-click does not
  // create two events.
  itemize.all("form").forEach(function (form) {
    form.addEventListener("submit", function () {
      itemize.all("button[type=submit], button:not([type])", form).forEach(
        function (btn) {
          btn.setAttribute("aria-busy", "true");
          // Disabling immediately would drop the button's name/value from the
          // submission, and the event form uses that to signal intent.
          setTimeout(function () {
            btn.disabled = true;
          }, 0);
        },
      );
    });
  });

  // Upgrade links to a confirmation page into an in-page dialog. Without
  // scripting the link is followed and the page does the same job.
  var dialog = document.getElementById("app-dialog");
  if (!dialog || typeof dialog.showModal !== "function") return;

  itemize.all("a[data-confirm]").forEach(function (link) {
    link.addEventListener("click", function (e) {
      e.preventDefault();
      confirmDialog(dialog, link.getAttribute("data-confirm"), function () {
        window.location.href = link.href;
      });
    });
  });
});

function confirmDialog(dialog, message, onAccept) {
  dialog.querySelector(".dialog__body").textContent = message;

  var accept = dialog.querySelector("[data-dialog-accept]");
  var cancel = dialog.querySelector("[data-dialog-cancel]");

  function cleanup() {
    accept.removeEventListener("click", onOK);
    cancel.removeEventListener("click", onCancel);
    dialog.close();
  }
  function onOK() {
    cleanup();
    onAccept();
  }
  function onCancel() {
    cleanup();
  }

  accept.addEventListener("click", onOK);
  cancel.addEventListener("click", onCancel);
  dialog.showModal();
}
