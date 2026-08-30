CREATE TABLE new_version_users (
  id VARCHAR(36) COLLATE utf8mb4_general_ci NOT NULL,
  unionid VARCHAR(64),
  phone VARCHAR(20) COLLATE utf8mb4_general_ci,
  member_level VARCHAR(20),
  member_expires_at DATETIME,
  updated_at DATETIME NOT NULL,
  is_deleted TINYINT NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE new_version_user_subscriptions (
  user_id VARCHAR(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  tier VARCHAR(20),
  expires_at DATETIME,
  monthly_chat_quota INT,
  current_period_used INT,
  updated_at DATETIME
);

CREATE TABLE new_version_memberships (
  id VARCHAR(36) COLLATE utf8mb4_general_ci NOT NULL,
  user_id VARCHAR(36) COLLATE utf8mb4_general_ci,
  phone VARCHAR(20) COLLATE utf8mb4_general_ci,
  status VARCHAR(20),
  start_date DATETIME,
  end_date DATETIME,
  consultation_limit INT,
  consultation_used INT,
  created_at DATETIME,
  updated_at DATETIME,
  PRIMARY KEY (id)
);

CREATE TABLE new_version_conversations (
  id VARCHAR(36) NOT NULL,
  user_id VARCHAR(36) COLLATE utf8mb4_general_ci NOT NULL,
  chat_mode VARCHAR(20),
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  is_deleted TINYINT NOT NULL,
  lesson_id VARCHAR(36),
  content_type VARCHAR(20),
  PRIMARY KEY (id)
);

CREATE TABLE new_version_messages (
  id VARCHAR(36) NOT NULL,
  user_id VARCHAR(36) COLLATE utf8mb4_general_ci NOT NULL,
  role VARCHAR(20),
  created_at DATETIME NOT NULL,
  is_deleted TINYINT NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE new_version_consultation_states (
  id VARCHAR(36) NOT NULL,
  session_id VARCHAR(36) NOT NULL,
  session_type VARCHAR(20),
  user_id VARCHAR(36) COLLATE utf8mb4_general_ci NOT NULL,
  created_at DATETIME,
  updated_at DATETIME,
  started_at DATETIME,
  ended_at DATETIME,
  is_deep_consult TINYINT,
  PRIMARY KEY (id)
);

CREATE TABLE new_version_assessments (
  id VARCHAR(36) NOT NULL,
  user_id VARCHAR(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  status VARCHAR(20),
  completed_at DATETIME,
  created_at DATETIME,
  updated_at DATETIME,
  PRIMARY KEY (id)
);

CREATE TABLE new_version_growth_reviews (
  id VARCHAR(36) NOT NULL,
  user_id VARCHAR(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  surfaced_at DATETIME,
  created_at DATETIME,
  PRIMARY KEY (id)
);

CREATE TABLE new_version_user_backgrounds (
  user_id VARCHAR(36) COLLATE utf8mb4_0900_ai_ci NOT NULL,
  focus_topics JSON,
  updated_at DATETIME,
  main_line_type VARCHAR(100),
  business_stage VARCHAR(100),
  pain_tag VARCHAR(100)
);

CREATE TABLE new_version_user_diagnoses (
  user_id VARCHAR(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  main_line_type VARCHAR(100),
  stage VARCHAR(100),
  updated_at DATETIME,
  user_segment VARCHAR(100)
);

CREATE TABLE new_version_user_interests (
  user_id VARCHAR(36) COLLATE utf8mb4_unicode_ci NOT NULL,
  interest_keys JSON,
  updated_at DATETIME
);
