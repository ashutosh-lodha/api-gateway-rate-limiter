CREATE TABLE IF NOT EXISTS request_logs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    api_key VARCHAR(100),
    endpoint VARCHAR(100),
    request_count INT,
    request_limit INT,
    status VARCHAR(20),
    timestamp BIGINT
);

CREATE TABLE IF NOT EXISTS api_keys (
    id INT AUTO_INCREMENT PRIMARY KEY,
    api_key VARCHAR(100) UNIQUE,
    plan VARCHAR(50),
    request_limit INT
);

INSERT INTO api_keys (api_key, plan, request_limit)
VALUES 
('user123', 'free', 5),
('proUser', 'pro', 20);