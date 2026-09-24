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

// renderDocument draws one text of an entry: the html the site rendered from it, and the text it was
// written as when the site answered no html.
//
// The html is written into the page as html, which is the whole point of it, so what makes that safe
// sits on the other side: the site renders a body with raw html left out, so an element a session
// wrote arrives as words rather than as an element. The fallback draws the text escaped, the way the
// page drew every body before it was rendered, so an answer without the html still reads.
function renderDocument(html, text) {
  if (html) { return '<div class="document">' + html + "</div>"; }
  if (!text) { return ""; }
  return '<pre class="text">' + esc(text) + "</pre>";
}

// STAGES_THAT_PLAY are the three stages whose artifact is a set of screens. The design system stage
// carries tokens, and the data model and the architecture carry their pictures inside their text, so
// none of those three has screens to play.
var STAGES_THAT_PLAY = ["discovery", "stories", "mockups"];

// renderFlowMap opens the flow map on a stage that holds screens, and draws nothing on one that does
// not, so an operator never presses something that leads nowhere.
//
// The address is relative to the project, the way the document beside it is, so the frame works at
// whatever address the project has. The page inside the frame then asks that stage for its screens.
function renderFlowMap(entry, row) {
  if (STAGES_THAT_PLAY.indexOf(entry) < 0) { return ""; }
  if (!row || !row.artifact) { return ""; }
  var at = esc(entry) + "/map/";
  var named = esc(ENTRY_NAMES[entry] || entry);
  return '<iframe class="map" src="' + at + '" title="The screens of the ' + named + ' stage"></iframe>' +
    '<p class="wide"><a href="' + at + '">Open the screens on their own</a></p>';
}

// bodyClass widens the column when the stage plays its screens. Prose is read at a width a person
// reads comfortably, and a map is looked at, so the two want different room.
function bodyClass(map) {
  return map ? "body plays" : "body";
}

// renderBody draws what one entry shows when it is chosen. A body is markdown, so the operator reads
// a document: its headings, its lists, its tables and its code, rather than the marks that make them.
function renderBody(answer, entry) {
  var head = '<h1>' + esc(ENTRY_NAMES[entry] || entry) + "</h1>";
  if (entry === "design") {
    var design = (answer || {}).design || {};
    var brief = design.brief_html
      ? '<div class="brief">' + design.brief_html + "</div>"
      : '<p class="brief">' + esc(design.brief) + "</p>";
    return '<article class="body" data-entry="design">' + head + brief +
      renderDocument(design.body_html, design.body) + "</article>";
  }
  var row = rowsByStage(answer)[entry];
  var state = stageState(row);
  var map = renderFlowMap(entry, row);
  if (!row || !row.body) {
    return '<article class="' + bodyClass(map) + '" data-entry="' + esc(entry) + '">' + head +
      '<p class="empty">This stage is ' + esc(state) + ".</p>" + map + "</article>";
  }
  return '<article class="' + bodyClass(map) + '" data-entry="' + esc(entry) + '">' + head +
    '<p class="state">' + esc(state) + " at version " + esc(row.version) + "</p>" + map +
    renderDocument(row.body_html, row.body) + "</article>";
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

  var libraryReady = false;

  // drawDiagrams turns every diagram in the stage into a picture, with the library this site serves.
  // It runs after a body is in the document, because a diagram is an element of that body and the
  // library is handed the elements rather than told to go and look for them.
  //
  // A page whose library did not arrive still reads: a diagram is a block of text inside the body, so
  // what is lost is the picture and never the stage.
  function drawDiagrams() {
    if (typeof mermaid === "undefined") { return; }
    var blocks = stage.querySelectorAll("pre.mermaid");
    if (!blocks.length) { return; }
    if (!libraryReady) {
      mermaid.initialize({ startOnLoad: false });
      libraryReady = true;
    }
    mermaid.run({ nodes: blocks }).catch(function (err) {
      console.error("a diagram in this stage could not be drawn", err);
    });
  }

  function show(entry) {
    showing = entry;
    stage.innerHTML = renderBody(answer, entry);
    Array.prototype.forEach.call(menu.querySelectorAll(".entry"), function (button) {
      button.setAttribute("aria-current", String(button.getAttribute("data-entry") === entry));
    });
    drawDiagrams();
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
