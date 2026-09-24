// The design site. It reads stages.json and draws one page per project: a menu of the design and the
// six stages, each with the one word that says where it stands, and the text of whichever entry the
// operator chose.
//
// The block between the two marks draws and nothing else. It touches no document and no global state,
// so a test runs it outside a browser and reads the markup an operator sees. Everything below the
// closing mark is the page around it: the fetch, the clicks and the drawing into the document.

// site:render:start

// STAGE_ORDER is the six stages in the order they are written, which is the order of their positions.
// It is here rather than read off the rows because a stage nobody wrote has no row and therefore no
// position, and the operator still has to see that it is to come. A test holds this list against the
// one the store writes.
var STAGE_ORDER = ["discovery", "stories", "design_system", "mockups", "data_model", "architecture"];

// The words in the menu. The design is an entry too, and it comes first, because everything under it
// is written to answer it.
var ENTRY_NAMES = {
  design: "Design",
  discovery: "Discovery",
  stories: "Stories",
  design_system: "Design system",
  mockups: "Mockups",
  data_model: "Data model",
  architecture: "Architecture"
};

// The five states, in the words a person reads. Changed since approval is the one this page exists
// for: the operator agreed to a text, and the text moved underneath the word.
var STATES = {
  approved: "approved",
  changed: "changed since approval",
  written: "written",
  unwritten: "not written",
  skipped: "skipped"
};

function esc(s) {
  return String(s === null || s === undefined ? "" : s)
    .replace(/[&<>"]/g, function (c) { return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; });
}

// stageState is where one stage stands, from the two version numbers on its row and the word skipped.
// A stage with no row was never written at all, which is why a missing row is a state rather than a
// gap in the menu.
function stageState(row) {
  if (!row) { return STATES.unwritten; }
  if (row.skipped) { return STATES.skipped; }
  if (!row.version) { return STATES.unwritten; }
  if (row.approved_version === row.version) { return STATES.approved; }
  if (row.approved_version) { return STATES.changed; }
  return STATES.written;
}

// designState is the same question asked of the design itself. The design carries no version and no
// approval in this document, so it reads written or not written and never the other three.
function designState(design) {
  return (design && (design.brief || design.body)) ? STATES.written : STATES.unwritten;
}

function rowsByStage(answer) {
  var rows = {};
  (((answer || {}).stages) || []).forEach(function (row) { rows[row.stage] = row; });
  return rows;
}

function renderEntry(entry, state) {
  return '<button type="button" class="entry" data-entry="' + esc(entry) + '" data-state="' + esc(state) + '">' +
    '<span class="name">' + esc(ENTRY_NAMES[entry] || entry) + "</span>" +
    '<span class="state">' + esc(state) + "</span></button>";
}

// renderMenu draws the whole menu: the design, then the six stages in order, each with one state.
function renderMenu(answer) {
  var rows = rowsByStage(answer);
  var entries = [renderEntry("design", designState((answer || {}).design))];
  STAGE_ORDER.forEach(function (stage) {
    entries.push(renderEntry(stage, stageState(rows[stage])));
  });
  return '<nav class="menu" aria-label="The design and its stages">' + entries.join("") + "</nav>";
}

// renderBody draws what one entry shows when it is chosen. The text is the text the session wrote,
// shown as text: markup inside a body is read rather than run.
function renderBody(answer, entry) {
  var head = '<h1>' + esc(ENTRY_NAMES[entry] || entry) + "</h1>";
  if (entry === "design") {
    var design = (answer || {}).design || {};
    return '<article class="body" data-entry="design">' + head +
      '<p class="brief">' + esc(design.brief) + "</p>" +
      '<pre class="text">' + esc(design.body) + "</pre></article>";
  }
  var row = rowsByStage(answer)[entry];
  var state = stageState(row);
  if (!row || !row.body) {
    return '<article class="body" data-entry="' + esc(entry) + '">' + head +
      '<p class="empty">This stage is ' + esc(state) + ".</p></article>";
  }
  return '<article class="body" data-entry="' + esc(entry) + '">' + head +
    '<p class="state">' + esc(state) + " at version " + esc(row.version) + "</p>" +
    '<pre class="text">' + esc(row.body) + "</pre></article>";
}

// site:render:end

// The page. It reads the document beside it, draws the menu, and shows whichever entry is chosen.
// The address of the document is relative, so the page works at whatever address the project has.
(function () {
  if (typeof document === "undefined") { return; }

  var menu = document.getElementById("menu");
  var stage = document.getElementById("stage");
  var answer = null;
  var showing = "design";

  function show(entry) {
    showing = entry;
    stage.innerHTML = renderBody(answer, entry);
    Array.prototype.forEach.call(menu.querySelectorAll(".entry"), function (button) {
      button.setAttribute("aria-current", String(button.getAttribute("data-entry") === entry));
    });
  }

  function draw() {
    menu.innerHTML = renderMenu(answer);
    Array.prototype.forEach.call(menu.querySelectorAll(".entry"), function (button) {
      button.addEventListener("click", function () { show(button.getAttribute("data-entry")); });
    });
    show(showing);
  }

  fetch("stages.json")
    .then(function (response) {
      if (!response.ok) { throw new Error("the site answered " + response.status); }
      return response.json();
    })
    .then(function (read) { answer = read; draw(); })
    .catch(function (err) {
      stage.innerHTML = '<p class="empty">This project\'s stages could not be read: ' + esc(err.message) + "</p>";
    });
})();
