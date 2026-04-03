ALTER TABLE sessions ADD COLUMN evaluation_decision TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN evaluation_reasoning TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN last_evaluated_at TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS evaluation_results (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id INTEGER NOT NULL,
  decision TEXT NOT NULL,
  confidence REAL NOT NULL,
  reasoning TEXT NOT NULL DEFAULT '',
  missing_elements_json TEXT NOT NULL DEFAULT '[]',
  clarifying_questions_json TEXT NOT NULL DEFAULT '[]',
  implementation_notes_json TEXT NOT NULL DEFAULT '[]',
  suggested_scope TEXT NOT NULL DEFAULT '',
  out_of_scope_assumptions_json TEXT NOT NULL DEFAULT '[]',
  raw_json TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS opencode_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id INTEGER NOT NULL,
  task_type TEXT NOT NULL,
  status TEXT NOT NULL,
  prompt_sha256 TEXT NOT NULL DEFAULT '',
  stdout_text TEXT NOT NULL DEFAULT '',
  stderr_text TEXT NOT NULL DEFAULT '',
  error_text TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
