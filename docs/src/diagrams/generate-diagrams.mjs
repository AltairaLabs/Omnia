#!/usr/bin/env node
/**
 * Generates .excalidraw JSON files for all diagrams.
 * Run: node docs/src/diagrams/generate-diagrams.mjs
 */
import { writeFileSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));

let idCounter = 0;
function uid() {
  return `elem_${++idCounter}_${Math.random().toString(36).slice(2, 10)}`;
}
function seed() {
  return Math.floor(Math.random() * 2000000000);
}

// Colors
const BLUE = "#a5d8ff";
const GREEN = "#b2f2bb";
const YELLOW = "#ffec99";
const ORANGE = "#ffc9c9";
const PURPLE = "#d0bfff";
const STROKE = "#1e1e1e";
const GRAY = "#e9ecef";

function baseElement(overrides) {
  return {
    id: uid(),
    angle: 0,
    strokeColor: STROKE,
    backgroundColor: "transparent",
    fillStyle: "solid",
    strokeWidth: 2,
    strokeStyle: "solid",
    roughness: 1,
    opacity: 100,
    groupIds: [],
    frameId: null,
    roundness: null,
    seed: seed(),
    version: 1,
    versionNonce: seed(),
    isDeleted: false,
    boundElements: null,
    updated: Date.now(),
    link: null,
    locked: false,
    ...overrides,
  };
}

function rect(x, y, w, h, bg = BLUE, extra = {}) {
  return baseElement({
    type: "rectangle",
    x,
    y,
    width: w,
    height: h,
    backgroundColor: bg,
    roundness: { type: 3 },
    ...extra,
  });
}

function text(x, y, t, fontSize = 16, extra = {}) {
  const lines = t.split("\n");
  const lineHeight = 1.25;
  const h = lines.length * fontSize * lineHeight;
  const maxLineLen = Math.max(...lines.map((l) => l.length));
  const w = maxLineLen * fontSize * 0.6;
  return baseElement({
    type: "text",
    x,
    y,
    width: w,
    height: h,
    text: t,
    fontSize,
    fontFamily: 1,
    textAlign: "center",
    verticalAlign: "middle",
    containerId: null,
    originalText: t,
    autoResize: true,
    lineHeight,
    ...extra,
  });
}

function arrow(x, y, points, extra = {}) {
  const xs = points.map((p) => p[0]);
  const ys = points.map((p) => p[1]);
  const w = Math.max(...xs) - Math.min(...xs);
  const h = Math.max(...ys) - Math.min(...ys);
  return baseElement({
    type: "arrow",
    x,
    y,
    width: w || 1,
    height: h || 1,
    points,
    lastCommittedPoint: null,
    startBinding: null,
    endBinding: null,
    startArrowhead: null,
    endArrowhead: "arrow",
    roundness: { type: 2 },
    ...extra,
  });
}

function line(x, y, points, extra = {}) {
  return arrow(x, y, points, {
    type: "line",
    startArrowhead: null,
    endArrowhead: null,
    ...extra,
  });
}

function boundTextInRect(r, label, fontSize = 16) {
  const t = text(0, 0, label, fontSize);
  t.containerId = r.id;
  t.textAlign = "center";
  t.verticalAlign = "middle";
  // Position centered in rect
  t.x = r.x + (r.width - t.width) / 2;
  t.y = r.y + (r.height - t.height) / 2;
  r.boundElements = [{ id: t.id, type: "text" }];
  return t;
}

function makeExcalidraw(elements) {
  return {
    type: "excalidraw",
    version: 2,
    source: "https://excalidraw.com",
    elements,
    appState: {
      gridSize: null,
      viewBackgroundColor: "#ffffff",
    },
    files: {},
  };
}

function writeExcalidraw(name, elements) {
  const path = join(__dirname, `${name}.excalidraw`);
  writeFileSync(path, JSON.stringify(makeExcalidraw(elements), null, 2));
  console.log(`  ✓ ${name}.excalidraw`);
}

// ─────────────────────────────────────────────────
// Diagram 1: ArenaSource Card
// ─────────────────────────────────────────────────
function arenaSourceCard() {
  const elements = [];

  // Title bar
  const titleBar = rect(0, 0, 520, 50, BLUE);
  elements.push(titleBar);
  const titleText = boundTextInRect(titleBar, "ArenaSource", 20);
  elements.push(titleText);

  // Body
  const body = rect(0, 50, 520, 120, GRAY);
  elements.push(body);

  // Bullet items
  const items = [
    "Git repository (branch, tag, or commit)",
    "OCI registry (container image format)",
    "Kubernetes ConfigMap (for simple cases)",
  ];
  items.forEach((item, i) => {
    const t = text(30, 62 + i * 32, `• ${item}`, 14, { textAlign: "left" });
    elements.push(t);
  });

  // Footer
  const footer = rect(0, 170, 520, 60, YELLOW);
  elements.push(footer);
  const footerLines = [
    "Polls source at interval → Updates artifact revision",
    "Provides download URL for workers",
  ];
  footerLines.forEach((l, i) => {
    const t = text(30, 180 + i * 24, l, 13, { textAlign: "left" });
    elements.push(t);
  });

  writeExcalidraw("arena-source-card", elements);
}

// ─────────────────────────────────────────────────
// Diagram 2: Arena Controllers Architecture
// ─────────────────────────────────────────────────
function arenaControllers() {
  const elements = [];

  // Outer container
  const container = rect(0, 0, 700, 420, "transparent", { strokeWidth: 2 });
  elements.push(container);
  const containerLabel = text(250, 10, "Arena Controllers", 22, {
    textAlign: "center",
  });
  elements.push(containerLabel);

  // Row 1: Three controllers
  const colW = 180;
  const colH = 60;
  const gap = 40;
  const startX = 50;
  const row1Y = 70;

  const srcCtrl = rect(startX, row1Y, colW, colH, BLUE);
  elements.push(srcCtrl);
  elements.push(boundTextInRect(srcCtrl, "ArenaSource\nController", 14));

  const cfgCtrl = rect(startX + colW + gap, row1Y, colW, colH, BLUE);
  elements.push(cfgCtrl);
  elements.push(boundTextInRect(cfgCtrl, "ArenaConfig\nController", 14));

  const jobCtrl = rect(startX + 2 * (colW + gap), row1Y, colW, colH, BLUE);
  elements.push(jobCtrl);
  elements.push(boundTextInRect(jobCtrl, "ArenaJob\nController", 14));

  // Arrows between controllers
  elements.push(
    arrow(startX + colW, row1Y + colH / 2, [
      [0, 0],
      [gap, 0],
    ])
  );
  elements.push(
    arrow(startX + 2 * colW + gap, row1Y + colH / 2, [
      [0, 0],
      [gap, 0],
    ])
  );

  // Row 2: Fetcher and Work Queue
  const row2Y = 190;
  const fetcher = rect(startX, row2Y, colW, colH, GREEN);
  elements.push(fetcher);
  elements.push(boundTextInRect(fetcher, "Fetcher\n(Git / OCI)", 14));

  const queue = rect(startX + 2 * (colW + gap), row2Y, colW, colH, GREEN);
  elements.push(queue);
  elements.push(boundTextInRect(queue, "Work Queue\n(Redis)", 14));

  // Vertical arrows from controllers to row 2
  elements.push(
    arrow(startX + colW / 2, row1Y + colH, [
      [0, 0],
      [0, row2Y - row1Y - colH],
    ])
  );
  elements.push(
    arrow(startX + 2 * (colW + gap) + colW / 2, row1Y + colH, [
      [0, 0],
      [0, row2Y - row1Y - colH],
    ])
  );

  // Row 3: Artifacts and Workers
  const row3Y = 310;
  const artifacts = rect(startX, row3Y, colW, colH, YELLOW);
  elements.push(artifacts);
  elements.push(boundTextInRect(artifacts, "Artifacts\nStorage", 14));

  const workers = rect(startX + 2 * (colW + gap), row3Y, colW, colH, YELLOW);
  elements.push(workers);
  elements.push(boundTextInRect(workers, "Workers\n(Pods)", 14));

  // Vertical arrows from row 2 to row 3
  elements.push(
    arrow(startX + colW / 2, row2Y + colH, [
      [0, 0],
      [0, row3Y - row2Y - colH],
    ])
  );
  elements.push(
    arrow(startX + 2 * (colW + gap) + colW / 2, row2Y + colH, [
      [0, 0],
      [0, row3Y - row2Y - colH],
    ])
  );

  writeExcalidraw("arena-controllers", elements);
}

// ─────────────────────────────────────────────────
// Diagram 3: GitOps Workflow
// ─────────────────────────────────────────────────
function arenaGitopsFlow() {
  const elements = [];
  const boxW = 130;
  const boxH = 50;
  const gap = 50;

  const labels = [
    { label: "Developer", bg: PURPLE },
    { label: "Git Push", bg: BLUE },
    { label: "ArenaSource", bg: GREEN },
    { label: "ArenaJob", bg: YELLOW },
    { label: "Results", bg: ORANGE },
  ];

  labels.forEach(({ label, bg }, i) => {
    const x = i * (boxW + gap);
    const r = rect(x, 40, boxW, boxH, bg);
    elements.push(r);
    elements.push(boundTextInRect(r, label, 14));

    if (i > 0) {
      elements.push(
        arrow(x - gap, 40 + boxH / 2, [
          [0, 0],
          [gap, 0],
        ])
      );
    }
  });

  // Artifact Update branch from ArenaSource
  const srcX = 2 * (boxW + gap);
  const branchY = 40 + boxH + 20;
  elements.push(
    arrow(srcX + boxW / 2, 40 + boxH, [
      [0, 0],
      [0, 40],
    ])
  );
  const artifactBox = rect(srcX - 10, branchY + 40, boxW + 20, 40, GRAY);
  elements.push(artifactBox);
  elements.push(boundTextInRect(artifactBox, "Artifact Update", 13));

  writeExcalidraw("arena-gitops-flow", elements);
}

// ─────────────────────────────────────────────────
// Diagram 4: Tilt Dev Workflow
// ─────────────────────────────────────────────────
function tiltDevWorkflow() {
  const elements = [];

  // Editor section
  const editorBox = rect(0, 0, 650, 160, GRAY, { strokeStyle: "dashed" });
  elements.push(editorBox);
  elements.push(text(270, 8, "Your Editor", 18));

  const dashSrc = rect(40, 50, 200, 80, BLUE);
  elements.push(dashSrc);
  elements.push(boundTextInRect(dashSrc, "dashboard/src/\n(TypeScript)", 14));

  const goSrc = rect(360, 50, 200, 80, GREEN);
  elements.push(goSrc);
  elements.push(boundTextInRect(goSrc, "internal/\n(Go)", 14));

  // Arrows down with labels
  elements.push(
    arrow(140, 160, [
      [0, 0],
      [0, 60],
    ])
  );
  elements.push(
    text(30, 172, "live_update\n(instant sync)", 12, { textAlign: "center" })
  );

  elements.push(
    arrow(460, 160, [
      [0, 0],
      [0, 60],
    ])
  );
  elements.push(
    text(370, 172, "docker_build\n(rebuild image)", 12, { textAlign: "center" })
  );

  // K8s cluster section
  const clusterBox = rect(0, 240, 650, 160, GRAY, { strokeStyle: "dashed" });
  elements.push(clusterBox);
  elements.push(text(200, 248, "Kubernetes Cluster (kind)", 18));

  const dashPod = rect(40, 290, 200, 80, BLUE);
  elements.push(dashPod);
  elements.push(
    boundTextInRect(dashPod, "Dashboard Pod\n(npm run dev)\n:3000", 13)
  );

  const opPod = rect(360, 290, 200, 80, GREEN);
  elements.push(opPod);
  elements.push(
    boundTextInRect(opPod, "Operator Pod\n(controller)\n:8082", 13)
  );

  // Arrow between pods
  elements.push(
    arrow(240, 330, [
      [0, 0],
      [120, 0],
    ])
  );

  // Port-forward arrows
  elements.push(
    arrow(140, 400, [
      [0, 0],
      [0, 50],
    ])
  );
  elements.push(text(70, 416, "port-forward", 12));
  elements.push(text(60, 458, "localhost:3000", 14, { textAlign: "center" }));

  elements.push(
    arrow(460, 400, [
      [0, 0],
      [0, 50],
    ])
  );
  elements.push(text(390, 416, "port-forward", 12));
  elements.push(text(380, 458, "localhost:8082", 14, { textAlign: "center" }));

  writeExcalidraw("tilt-dev-workflow", elements);
}

// ─────────────────────────────────────────────────
// Diagram 5: ArenaDevSession Sequence
// ─────────────────────────────────────────────────
function arenaDevSessionSequence() {
  const elements = [];

  // Three actors
  const actorW = 160;
  const actorH = 40;
  const colGap = 100;
  const col1X = 0;
  const col2X = actorW + colGap;
  const col3X = 2 * (actorW + colGap);

  const actor1 = rect(col1X, 0, actorW, actorH, PURPLE);
  elements.push(actor1);
  elements.push(boundTextInRect(actor1, "Project Editor", 14));

  const actor2 = rect(col2X, 0, actorW, actorH, BLUE);
  elements.push(actor2);
  elements.push(boundTextInRect(actor2, "ArenaDevSession", 14));

  const actor3 = rect(col3X, 0, actorW, actorH, GREEN);
  elements.push(actor3);
  elements.push(boundTextInRect(actor3, "Dev Console Pod", 14));

  // Lifelines
  const lineStartY = actorH;
  const lineEndY = 380;
  [col1X, col2X, col3X].forEach((x) => {
    elements.push(
      line(x + actorW / 2, lineStartY, [
        [0, 0],
        [0, lineEndY - lineStartY],
      ], { strokeStyle: "dashed", strokeWidth: 1 })
    );
  });

  // Messages
  const msgs = [
    {
      fromCol: 0,
      toCol: 1,
      y: 70,
      label: 'Click "Test Agent"',
    },
    {
      fromCol: 1,
      toCol: 2,
      y: 120,
      label: "Create Pod + Service",
    },
    {
      fromCol: 2,
      toCol: 1,
      y: 170,
      label: "Status: Ready",
      dashed: true,
    },
    {
      fromCol: 1,
      toCol: 0,
      y: 220,
      label: "WebSocket URL",
      dashed: true,
    },
    {
      fromCol: 0,
      toCol: 2,
      y: 280,
      label: "WebSocket Connection",
      double: true,
    },
    {
      fromCol: 1,
      toCol: 2,
      y: 340,
      label: "Delete (idle timeout)",
    },
  ];

  const cols = [
    col1X + actorW / 2,
    col2X + actorW / 2,
    col3X + actorW / 2,
  ];

  msgs.forEach(({ fromCol, toCol, y, label, dashed, double }) => {
    const fromX = cols[fromCol];
    const toX = cols[toCol];
    const dx = toX - fromX;

    const a = arrow(fromX, y, [
      [0, 0],
      [dx, 0],
    ], {
      strokeStyle: dashed ? "dashed" : "solid",
      strokeWidth: double ? 3 : 2,
    });
    elements.push(a);

    const labelX = Math.min(fromX, toX) + Math.abs(dx) / 2;
    const t = text(labelX - 60, y - 20, label, 12, { textAlign: "center" });
    elements.push(t);
  });

  writeExcalidraw("arena-dev-session-sequence", elements);
}

// ─────────────────────────────────────────────────
// Diagram 6: ArenaJob Flow
// ─────────────────────────────────────────────────
function arenaJobFlow() {
  const elements = [];
  const boxW = 120;
  const boxH = 50;
  const gap = 50;

  // Main flow: ArenaConfig → ArenaJob → Workers → Results
  const mainFlow = [
    { label: "ArenaConfig", bg: BLUE },
    { label: "ArenaJob", bg: GREEN },
    { label: "Workers", bg: YELLOW },
    { label: "Results", bg: ORANGE },
  ];

  mainFlow.forEach(({ label, bg }, i) => {
    const x = i * (boxW + gap);
    const r = rect(x, 0, boxW, boxH, bg);
    elements.push(r);
    elements.push(boundTextInRect(r, label, 14));
    if (i > 0) {
      elements.push(
        arrow(x - gap, boxH / 2, [
          [0, 0],
          [gap, 0],
        ])
      );
    }
  });

  // Branch from ArenaJob
  const jobX = 1 * (boxW + gap);
  const branchStartY = boxH;

  // Progress tracking
  elements.push(
    arrow(jobX + boxW / 2 - 20, branchStartY, [
      [0, 0],
      [0, 40],
    ])
  );
  const progressBox = rect(jobX - 30, boxH + 45, boxW + 20, 35, GRAY);
  elements.push(progressBox);
  elements.push(boundTextInRect(progressBox, "Progress tracking", 12));

  // Output storage
  elements.push(
    arrow(jobX + boxW / 2 + 20, branchStartY, [
      [0, 0],
      [0, 80],
    ])
  );
  const outputBox = rect(jobX - 20, boxH + 90, boxW + 30, 35, GRAY);
  elements.push(outputBox);
  elements.push(boundTextInRect(outputBox, "Output storage", 12));

  writeExcalidraw("arenajob-flow", elements);
}

// ─────────────────────────────────────────────────
// Diagram 7: Arena Getting Started Flow
// ─────────────────────────────────────────────────
function arenaGettingStartedFlow() {
  const elements = [];
  const boxW = 130;
  const boxH = 50;
  const gap = 50;

  const flow = [
    { label: "ArenaSource", bg: BLUE },
    { label: "ArenaConfig", bg: GREEN },
    { label: "ArenaJob", bg: YELLOW },
    { label: "Results", bg: ORANGE },
  ];

  flow.forEach(({ label, bg }, i) => {
    const x = i * (boxW + gap);
    const r = rect(x, 0, boxW, boxH, bg);
    elements.push(r);
    elements.push(boundTextInRect(r, label, 14));
    if (i > 0) {
      elements.push(
        arrow(x - gap, boxH / 2, [
          [0, 0],
          [gap, 0],
        ])
      );
    }
  });

  // Descriptions below each box
  const descriptions = [
    "Fetches your\nPromptKit bundle",
    "Defines what to\ntest and how",
    "Executes the\nevaluation",
    "",
  ];

  descriptions.forEach((desc, i) => {
    if (!desc) return;
    const x = i * (boxW + gap);
    elements.push(
      arrow(x + boxW / 2, boxH, [
        [0, 0],
        [0, 20],
      ], { strokeWidth: 1 })
    );
    elements.push(text(x + 5, boxH + 25, desc, 12, { textAlign: "center" }));
  });

  writeExcalidraw("arena-getting-started-flow", elements);
}

// ─────────────────────────────────────────────────
// Diagram 8: ArenaConfig Flow
// ─────────────────────────────────────────────────
function arenaConfigFlow() {
  const elements = [];

  // ArenaSource and Provider(s) feeding into ArenaConfig
  const srcBox = rect(0, 0, 140, 50, BLUE);
  elements.push(srcBox);
  elements.push(boundTextInRect(srcBox, "ArenaSource", 14));

  const provBox = rect(0, 80, 140, 50, GREEN);
  elements.push(provBox);
  elements.push(boundTextInRect(provBox, "Provider(s)", 14));

  // Arrows converging
  elements.push(
    arrow(140, 25, [
      [0, 0],
      [60, 30],
    ])
  );
  elements.push(
    arrow(140, 105, [
      [0, 0],
      [60, -30],
    ])
  );

  // ArenaConfig
  const cfgBox = rect(220, 30, 150, 60, YELLOW);
  elements.push(cfgBox);
  elements.push(boundTextInRect(cfgBox, "ArenaConfig", 16));

  // Arrow to ArenaJob
  elements.push(
    arrow(370, 60, [
      [0, 0],
      [50, 0],
    ])
  );

  const jobBox = rect(440, 30, 130, 60, ORANGE);
  elements.push(jobBox);
  elements.push(boundTextInRect(jobBox, "ArenaJob", 16));

  // Arrow to Results
  elements.push(
    arrow(570, 60, [
      [0, 0],
      [50, 0],
    ])
  );

  const resBox = rect(640, 35, 110, 50, PURPLE);
  elements.push(resBox);
  elements.push(boundTextInRect(resBox, "Results", 16));

  writeExcalidraw("arenaconfig-flow", elements);
}

// ─────────────────────────────────────────────────
// Diagram 9: Arena Template Source Flow
// ─────────────────────────────────────────────────
function arenaTemplateSourceFlow() {
  const elements = [];
  const boxW = 160;
  const boxH = 50;
  const gap = 50;

  const flow = [
    { label: "ArenaTemplate\nSource", bg: BLUE },
    { label: "Fetch", bg: GREEN },
    { label: "Discover", bg: YELLOW },
    { label: "Templates\nReady", bg: ORANGE },
  ];

  flow.forEach(({ label, bg }, i) => {
    const x = i * (boxW + gap);
    const r = rect(x, 0, boxW, boxH, bg);
    elements.push(r);
    elements.push(boundTextInRect(r, label, 14));
    if (i > 0) {
      elements.push(
        arrow(x - gap, boxH / 2, [
          [0, 0],
          [gap, 0],
        ])
      );
    }
  });

  // Branch from Discover
  const discX = 2 * (boxW + gap);
  elements.push(
    arrow(discX + boxW / 2, boxH, [
      [0, 0],
      [0, 40],
    ])
  );
  const metaBox = rect(discX + 10, boxH + 45, boxW - 20, 40, GRAY);
  elements.push(metaBox);
  elements.push(
    boundTextInRect(metaBox, "template.yaml\nmetadata", 12)
  );

  writeExcalidraw("arena-template-source-flow", elements);
}

// ─────────────────────────────────────────────────
// Generate all
// ─────────────────────────────────────────────────
console.log("Generating .excalidraw files...");
arenaSourceCard();
arenaControllers();
arenaGitopsFlow();
tiltDevWorkflow();
arenaDevSessionSequence();
arenaJobFlow();
arenaGettingStartedFlow();
arenaConfigFlow();
arenaTemplateSourceFlow();
console.log("Done! Generated 9 .excalidraw files.");
