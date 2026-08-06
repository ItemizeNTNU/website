/*
 * The command line.
 *
 * An enhancement, never the only way anywhere: everything it reaches is also a
 * tab. It renders only under html[data-js], and it is deliberately not
 * autofocused — autofocus takes keyboard scrolling away from someone who came
 * to read.
 *
 * Commands are small and honest. Nothing here fabricates system output.
 */

itemize.ready(function () {
  var form = document.getElementById("console-form");
  var input = document.getElementById("console-in");
  var out = document.getElementById("console-out");
  if (!form || !input || !out) return;

  var history = [];
  var historyAt = -1;

  form.addEventListener("submit", function (e) {
    e.preventDefault();
    var raw = input.value.trim();
    input.value = "";
    if (!raw) return;

    history.unshift(raw);
    historyAt = -1;

    echo(out, raw);
    run(raw, out);
    // FIX: Scroll the very last line/child inside your output element into view
    if (out.lastElementChild) {
      out.lastElementChild.scrollIntoView({ behavior: "smooth", block: "nearest" });
    }
  });

  // Up and down walk the history, as a shell does. These only fire while the
  // input has focus, so they never fight the page.
  input.addEventListener("keydown", function (e) {
    if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
    if (!history.length) return;
    e.preventDefault();

    if (e.key === "ArrowUp") {
      historyAt = Math.min(historyAt + 1, history.length - 1);
    } else {
      historyAt = Math.max(historyAt - 1, -1);
    }
    input.value = historyAt < 0 ? "" : history[historyAt];
  });
});

function echo(out, text) {
  var el = document.createElement("p");
  el.className = "console__echo";
  el.textContent = text;
  out.appendChild(el);
}

function say(out, text, kind) {
  var el = document.createElement("p");
  el.className = "console__line" + (kind ? " console__line--" + kind : "");
  el.textContent = text;
  out.appendChild(el);
  return el;
}

/* The pages the console knows about, read from the tabs so the two cannot
   drift apart. */
function pages() {
  return itemize.all("[data-tab]").map(function (tab) {
    return {
      name: tab.textContent.trim(),
      href: tab.getAttribute("href"),
    };
  });
}

function findPage(name) {
  var wanted = name.replace(/^\/+|\/+$/g, "").replace(/-/g, "_");
  return (
    pages().filter(function (p) {
      var n = p.name.replace(/^~$/, "").replace(/-/g, "_");
      return n === wanted || p.href === "/" + name || (wanted === "" && p.name === "~");
    })[0] || null
  );
}

var COMMANDS = {
  help: function (_, out) {
    say(out, "Kommandoer:");
    say(out, "  ls                 vis sidene");
    say(out, "  cd <side>          gå til en side");
    say(out, "  whoami             hvem du er logget inn som");
    say(out, "  date               klokka nå");
    say(out, "  clear              tøm konsollet");
    say(out, "Piltastene ← og → bytter fane.");
  },

  ls: function (_, out) {
    var line = say(out, "");
    pages().forEach(function (p, i) {
      if (i) line.appendChild(document.createTextNode("  "));
      var a = document.createElement("a");
      a.href = p.href;
      a.textContent = p.name;
      line.appendChild(a);
    });
  },

  cd: function (args, out) {
    if (!args.length) {
      window.location.href = "/";
      return;
    }
    var page = findPage(args[0]);
    if (!page) {
      say(out, "cd: finnes ikke: " + args[0], "error");
      return;
    }
    // A real navigation, not a fake one — the URL and history stay honest.
    window.location.href = page.href;
  },

  whoami: function (_, out) {
    var session = document.querySelector(".status__session");
    say(out, session ? session.textContent.trim() : "gjest");
  },

  date: function (_, out) {
    say(
      out,
      new Date().toLocaleString("nb-NO", { timeZone: "Europe/Oslo" }),
    );
  },

  clear: function (_, out) {
    out.textContent = "";
  },

  // Two for the audience this site is for.
  sudo: function (_, out) {
    say(out, "Pent forsøk.", "error");
  },

  flag: function (_, out) {
    say(out, "Ikke her. Prøv å lese kilden på /ressurser.");
  },
};

COMMANDS.cat = COMMANDS.cd;
COMMANDS.open = COMMANDS.cd;
COMMANDS.man = COMMANDS.help;

function run(raw, out) {
  var parts = raw.split(/\s+/);
  var name = parts[0].toLowerCase();
  var fn = Object.prototype.hasOwnProperty.call(COMMANDS, name)
    ? COMMANDS[name]
    : null;

  if (!fn) {
    // In the shell's own voice: what happened, and what to do about it.
    say(out, "kommando ikke funnet: " + parts[0], "error");
    say(out, "Skriv 'help' for å se hva som finnes.");
    return;
  }
  fn(parts.slice(1), out);
}
