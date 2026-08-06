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

  /*
   * Runs fn as soon as the page stops being visible, or straight away if it
   * already is not. Returns a function that cancels it.
   *
   * Every text animation here reveals content that is hidden until the
   * animation reaches it, and a backgrounded tab has its timers throttled to a
   * crawl and eventually frozen — so a run left half-finished leaves the page
   * half-drawn. Nobody is watching an animation they cannot see, so each of
   * them jumps to its end state instead and the page is whole on return.
   */
  function onHidden(fn) {
    if (document.visibilityState === "hidden") {
      fn();
      return function () {};
    }
    function handler() {
      if (document.visibilityState === "hidden") fn();
    }
    document.addEventListener("visibilitychange", handler);
    return function () {
      document.removeEventListener("visibilitychange", handler);
    };
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
    onHidden: onHidden,
    ready: ready,
    all: all,
  };
})();
