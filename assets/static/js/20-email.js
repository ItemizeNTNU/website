/*
 * Email addresses.
 *
 * No address appears as plain text anywhere in the served HTML, and there is
 * no mailto: href for a crawler to pick up. The server emits the address
 * rotated by thirteen places in data-eml; this reverses it, fills in the link
 * text, and attaches the real href.
 *
 * This is mitigation, not a cure. It defeats the harvesters that fetch HTML
 * and run a regular expression over it — which is the great majority of them,
 * and where the volume comes from — but a scraper driving a real browser still
 * sees the finished DOM. The <noscript> fallback keeps the address reachable
 * for people without scripting, written in a form that is readable to a human
 * but not to a pattern match.
 */

itemize.ready(function () {
  itemize.all("[data-eml]").forEach(function (el) {
    var address = rot13(el.getAttribute("data-eml"));
    if (!address) return;

    el.textContent = address;
    el.removeAttribute("data-eml");

    // aria-label on the anchor rather than the span so assistive technology
    // announces the finished address, never a half-resolved one while the
    // reveal animation is still running.
    var link = el.closest("a");
    if (link) {
      link.setAttribute("href", "mailto:" + address);
      link.setAttribute("aria-label", address);
    }
  });
});

function rot13(s) {
  if (!s) return "";
  return s.replace(/[a-zA-Z]/g, function (c) {
    var base = c <= "Z" ? 65 : 97;
    return String.fromCharCode(((c.charCodeAt(0) - base + 13) % 26) + base);
  });
}
