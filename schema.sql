CREATE TABLE IF NOT EXISTS giveaways (
    id TEXT PRIMARY KEY,
    title TEXT,
    end_time INTEGER,
    role_id TEXT,
    channel_id TEXT,
    message_id TEXT
    winners INTEGER DEFAULT 1
);

CREATE TABLE IF NOT EXISTS participants (
    giveaway_id TEXT,
    user_id TEXT,
    PRIMARY KEY (giveaway_id, user_id)
);
