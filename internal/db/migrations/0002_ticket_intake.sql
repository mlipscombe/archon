ALTER TABLE projects ADD COLUMN last_issue_updated TEXT NOT NULL DEFAULT '';

ALTER TABLE sessions ADD COLUMN issue_type TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN priority TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN jira_status TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN issue_updated_at TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS ticket_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id INTEGER NOT NULL,
  issue_key TEXT NOT NULL,
  issue_updated_at TEXT NOT NULL,
  normalized_text TEXT NOT NULL DEFAULT '',
  description_text TEXT NOT NULL DEFAULT '',
  acceptance_criteria_text TEXT NOT NULL DEFAULT '',
  comments_text TEXT NOT NULL DEFAULT '',
  labels_json TEXT NOT NULL DEFAULT '[]',
  components_json TEXT NOT NULL DEFAULT '[]',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE,
  UNIQUE(issue_key, issue_updated_at)
);
