CREATE TABLE channels (
	"id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id", "guild_id")
);

CREATE INDEX CONCURRENTLY channels_guild_id ON channels("guild_id");

CREATE TABLE guilds (
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id")
);

CREATE TABLE voice_states (
	"guild_id" int8 NOT NULL,
	"user_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("guild_id", "user_id")
);

CREATE TABLE members (
	"guild_id" int8 NOT NULL,
	"user_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("guild_id", "user_id")
);

CREATE INDEX CONCURRENTLY members_guild_id ON members("guild_id");
CREATE INDEX CONCURRENTLY members_user_id ON members("user_id");

CREATE TABLE messages (
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id")
);

CREATE TABLE roles (
	"id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id", "guild_id")
);

CREATE TABLE emojis (
	"id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id", "guild_id")
);
