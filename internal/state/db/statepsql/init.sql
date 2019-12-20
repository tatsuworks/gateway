CREATE TABLE channels (
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id")
);

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
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id")
);

CREATE TABLE messages (
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id")
);

CREATE TABLE roles (
	"id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY("id")
);
