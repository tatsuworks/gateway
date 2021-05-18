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
IF NOT EXISTS members_guild_id ON members
("guild_id");
CREATE INDEX CONCURRENTLY
IF NOT EXISTS members_user_id ON members
("user_id");

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

CREATE UNLOGGED TABLE
IF NOT EXISTS shards
(
	"id" int NOT NULL,
	"name" text NOT NULL,
	"seq" int8 NOT NULL,
	"sess" text NOT NULL,
	PRIMARY KEY
("id", "name")
);

CREATE TABLE "public"."guilds_persistent"
(
	"id" int8,
	PRIMARY KEY ("id")
);

CREATE OR REPLACE FUNCTION addGuildToPersistent
() RETURNS TRIGGER AS $joined_guild_trigger$
BEGIN
	INSERT INTO guilds_persistent
		(id)
	VALUES
		(NEW.id)
	ON CONFLICT
	(id) DO NOTHING;
	RETURN NEW;
END;
$joined_guild_trigger$ LANGUAGE plpgsql;
CREATE TRIGGER joined_guild AFTER
INSERT ON
guilds
FOR
EACH
ROW
EXECUTE PROCEDURE addGuildToPersistent
();

CREATE OR REPLACE FUNCTION removeGuildFromPersistent
() RETURNS TRIGGER AS $left_guild_trigger$
BEGIN
	delete from guilds_persistent where id = OLD.id;
	RETURN OLD;
END;
$left_guild_trigger$ LANGUAGE plpgsql;

CREATE TRIGGER left_guild AFTER
DELETE ON guilds FOR EACH
ROW
EXECUTE PROCEDURE removeGuildFromPersistent
();

CREATE UNLOGGED TABLE
IF NOT EXISTS threads
(
	"id" int8 NOT NULL,
	"parent_id" int8 NOT NULL,
	"guild_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY
("id")
);

CREATE INDEX CONCURRENTLY
IF NOT EXISTS threads_guild_id ON threads
("guild_id");
