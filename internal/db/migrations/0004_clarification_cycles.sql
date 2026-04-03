CREATE TABLE IF NOT EXISTS clarification_cycles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id INTEGER NOT NULL,
  evaluation_result_id INTEGER NOT NULL,
  jira_comment_id TEXT NOT NULL DEFAULT '',
  comment_body TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE,
  FOREIGN KEY(evaluation_result_id) REFERENCES evaluation_results(id) ON DELETE CASCADE
);
