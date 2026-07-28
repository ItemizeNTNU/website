/*
 * app.js — shared helpers.
 *
 * The source files in this directory are concatenated in filename order and
 * served as one script, so everything here is plain browser JavaScript in a
 * single scope. No bundler, no modules, no build step.
 */

var itemize = (function () {
  "use strict";

  /*
   * Every animation on this site checks this before starting. The CSS
   * reduced-motion block cannot reach a JavaScript-driven text animation, so
   * the guard has to exist in both places.
   */
  function prefersReducedMotion() {
    return (
      window.matchMedia &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    );
  }

  function ready(fn) {
    if (document.readyState !== "loading") {
      fn();
    } else {
      document.addEventListener("DOMContentLoaded", fn);
    }
  }

  function all(selector, root) {
    return Array.prototype.slice.call(
      (root || document).querySelectorAll(selector),
    );
  }

  return {
    prefersReducedMotion: prefersReducedMotion,
    ready: ready,
    all: all,
  };
})();
