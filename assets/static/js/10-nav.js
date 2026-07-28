/* Mobile navigation. */

itemize.ready(function () {
  var toggle = document.querySelector(".nav__toggle");
  var menu = document.getElementById("nav-menu");
  if (!toggle || !menu) return;

  function setOpen(open) {
    toggle.setAttribute("aria-expanded", open ? "true" : "false");
    if (open) {
      menu.setAttribute("data-open", "");
    } else {
      menu.removeAttribute("data-open");
    }
  }

  toggle.addEventListener("click", function () {
    setOpen(toggle.getAttribute("aria-expanded") !== "true");
  });

  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && toggle.getAttribute("aria-expanded") === "true") {
      setOpen(false);
      toggle.focus();
    }
  });

  // Following a link inside the menu should not leave it open behind the
  // next page's paint on a slow connection.
  menu.addEventListener("click", function (e) {
    if (e.target.closest("a")) setOpen(false);
  });
});
