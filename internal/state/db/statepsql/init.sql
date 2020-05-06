CREATE UNLOGGED TABLE IF NOT EXISTS channels (
	"id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id", "guild_id")
);

-- SELECT create_distributed_table('channels', 'guild_id');
CREATE INDEX CONCURRENTLY IF NOT EXISTS channels_guild_id ON channels("guild_id");

CREATE UNLOGGED TABLE IF NOT EXISTS guilds (
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id")
);

-- SELECT create_distributed_table('guilds', 'id');

CREATE UNLOGGED TABLE IF NOT EXISTS voice_states (
	"guild_id" int8 NOT NULL,
	"user_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("guild_id", "user_id")
);

-- SELECT create_distributed_table('voice_states', 'guild_id');

CREATE UNLOGGED TABLE IF NOT EXISTS members (
	"guild_id" int8 NOT NULL,
	"user_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("guild_id", "user_id")
);

-- SELECT create_distributed_table('members', 'guild_id');
CREATE INDEX CONCURRENTLY IF NOT EXISTS members_guild_id ON members("guild_id");
CREATE INDEX CONCURRENTLY IF NOT EXISTS members_user_id ON members("user_id");

CREATE UNLOGGED TABLE IF NOT EXISTS messages (
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id")
);

CREATE UNLOGGED TABLE IF NOT EXISTS roles (
	"id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id", "guild_id")
);

-- SELECT create_distributed_table('roles', 'guild_id');
CREATE INDEX CONCURRENTLY IF NOT EXISTS roles_guild_id ON roles("guild_id");

CREATE UNLOGGED TABLE IF NOT EXISTS emojis (
	"id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id", "guild_id")
);

-- SELECT create_distributed_table('emojis', 'guild_id');
CREATE INDEX CONCURRENTLY IF NOT EXISTS emojis_guild_id ON emojis("guild_id");

CREATE UNLOGGED TABLE IF NOT EXISTS shards (
	"id" int NOT NULL,
	"name" text NOT NULL,
	"seq" int8 NOT NULL,
	"sess" text NOT NULL,
	PRIMARY KEY("id", "name")
);
