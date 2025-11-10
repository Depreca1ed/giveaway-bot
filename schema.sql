CREATE TABLE IF NOT EXISTS giveaways (
    id TEXT,
    guild_id TEXT,
    title TEXT,
    end_time INTEGER,
    role_id TEXT,
    channel_id TEXT,
    message_id TEXT,
    winners INTEGER DEFAULT 1,
    PRIMARY KEY (id, guild_id)
);

CREATE TABLE IF NOT EXISTS participants (
    giveaway_id TEXT,
    guild_id TEXT,
    user_id TEXT,
    PRIMARY KEY (giveaway_id, guild_id, user_id)
);
