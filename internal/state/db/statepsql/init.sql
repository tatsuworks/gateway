CREATE UNLOGGED
TABLE
IF NOT EXISTS channels
(
	"id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY
("id", "guild_id")
);

-- SELECT create_distributed_table('channels', 'guild_id');
CREATE INDEX CONCURRENTLY
IF NOT EXISTS channels_guild_id ON channels
("guild_id");

CREATE UNLOGGED TABLE
IF NOT EXISTS guilds
(
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY
("id")
);

-- SELECT create_distributed_table('guilds', 'id');

CREATE UNLOGGED TABLE
IF NOT EXISTS voice_states
(
	"guild_id" int8 NOT NULL,
	"user_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY
("guild_id", "user_id")
);

-- SELECT create_distributed_table('voice_states', 'guild_id');

CREATE UNLOGGED TABLE
IF NOT EXISTS members
(
	"guild_id" int8 NOT NULL,
	"user_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	"last_updated" timestamp NOT NULL DEFAULT now
(),
	PRIMARY KEY
("guild_id", "user_id")
);

CREATE OR REPLACE FUNCTION update_last_updated_column
()
RETURNS TRIGGER AS $$
BEGIN
NEW.last_updated = now
();
RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_members_last_updated BEFORE
UPDATE ON members FOR EACH ROW
EXECUTE PROCEDURE update_last_updated_column
();


-- SELECT create_distributed_table('members', 'guild_id');
CREATE INDEX CONCURRENTLY
IF NOT EXISTS members_user_id ON members
("user_id");
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX CONCURRENTLY
IF NOT EXISTS members_search_trgm ON members
USING GIN (
	lower(
		coalesce(data->'user'->>'global_name', '') || ' ' ||
		coalesce(data->'user'->>'display_name', '') || ' ' ||
		coalesce(data->'user'->>'username', '') || ' ' ||
		coalesce(data->>'nick', '')
	) gin_trgm_ops
);

CREATE UNLOGGED TABLE
IF NOT EXISTS messages
(
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY
("id")
);

CREATE UNLOGGED TABLE
IF NOT EXISTS roles
(
	"id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY
("id", "guild_id")
);

-- SELECT create_distributed_table('roles', 'guild_id');
CREATE INDEX CONCURRENTLY
IF NOT EXISTS roles_guild_id ON roles
("guild_id");

CREATE UNLOGGED TABLE
IF NOT EXISTS emojis
(
	"id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY
("id", "guild_id")
);

-- SELECT create_distributed_table('emojis', 'guild_id');
CREATE INDEX CONCURRENTLY
IF NOT EXISTS emojis_guild_id ON emojis
("guild_id");

CREATE UNLOGGED TABLE IF NOT EXISTS shards
(
	"id" int NOT NULL,
	"name" text NOT NULL,
	"seq" int8 NOT NULL,
	"sess" text NOT NULL,
	"status" text NOT NULL DEFAULT '',
    "resume_url" text NOT NULL DEFAULT '',
	PRIMARY KEY ("id", "name")
);

CREATE UNLOGGED TABLE
IF NOT EXISTS threads
(
	"id" int8 NOT NULL,
	"owner_id" int8 NOT NULL,
	"parent_id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY
("id")
);

CREATE INDEX CONCURRENTLY
IF NOT EXISTS threads_guild_id ON threads
("guild_id");

CREATE UNLOGGED TABLE
IF NOT EXISTS presence
(
	"user_id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY
("user_id", "guild_id")
);

CREATE INDEX CONCURRENTLY
IF NOT EXISTS presence_guild_id ON presence
("guild_id");

CREATE UNLOGGED TABLE
IF NOT EXISTS guild_backfills
(
	"guild_id" int8 NOT NULL,
	"started_at" timestamp NOT NULL DEFAULT now(),
	"backfilled_at" timestamp NULL,
	PRIMARY KEY
("guild_id")
);
