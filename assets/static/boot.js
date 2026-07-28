/*
 * Loaded synchronously in <head>. Its only job is to tell the stylesheet that
 * scripting is available, before the first paint.
 *
 * The mobile navigation only collapses into a hamburger under html[data-js];
 * without this flag set early, the menu would render expanded and then snap
 * shut once the deferred bundle ran. It is a separate file rather than an
 * inline script because the Content-Security-Policy forbids inline script —
 * deliberately, since that is what makes the policy worth having.
 */
document.documentElement.setAttribute("data-js", "");
