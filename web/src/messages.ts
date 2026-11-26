// All user-facing copy lives here, so wording is reviewed in one place and a
// second language can be added later without touching the components.

export const messages = {
  appName: "Talent-Bridge",
  tagline: "Cricket technique, scored against pro players",
  nav: { label: "Main", settings: "Settings" },
  home: {
    heading: "Your technique, scored against the pros",
    intro:
      "Talent-Bridge scores a cricket clip against pro players for the same shot and camera view, then tells you the one thing to fix first.",
    stepsHeading: "How it will work",
    steps: [
      "Record a short clip of one shot or one ball. The clip stays on your device.",
      "Your browser finds your body position in every frame.",
      "The server compares each phase of your action with pro players of the same shot and view.",
      "You get a score, the faults that matter most, and a 4-week drill plan.",
    ],
    buildNote:
      "This is the first build. It connects the page, the Go server and the database; recording and scoring come in the next slices. Settings shows the live system status.",
  },
  settings: { heading: "Settings" },
  notFound: { heading: "Page not found", back: "Go to the home page" },
  status: {
    heading: "System status",
    server: "Server",
    database: "Database",
    schema: "Schema",
    build: "Build",
    serverUp: "Answering",
    serverDown: "Not reachable",
    dbUp: "Connected",
    dbDown: "Not reachable",
    latency: (ms: number) => `${ms} ms`,
    migration: (n: number) => `Migration ${n}`,
    notMeasured: "Not measured",
    checking: "Checking…",
    coldStart: "The first check can take a few seconds while the server and the database wake up.",
    checkAgain: "Check again",
    allGood: "All parts are connected.",
    dbDownHelp: "The server is up, but it cannot reach the database. Try again in a moment.",
    serverDownHelp: "The server did not answer. Check your connection, then try again.",
  },
} as const;
