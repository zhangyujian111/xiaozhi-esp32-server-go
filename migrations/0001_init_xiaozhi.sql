-- 0001_init_xiaozhi.sql
-- Initial schema for xiaozhi dialogue memory

CREATE TABLE IF NOT EXISTS xiaozhi_device (
    device_id VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS xiaozhi_session (
    session_id VARCHAR(64) PRIMARY KEY,
    device_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_active_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_device (device_id)
);

CREATE TABLE IF NOT EXISTS xiaozhi_message (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    device_id VARCHAR(64) NOT NULL,
    session_id VARCHAR(64) NOT NULL,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_device_session (device_id, session_id, created_at)
);
