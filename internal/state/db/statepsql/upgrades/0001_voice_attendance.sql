-- Voice presence, readiness and loss support for voice attendance (TATSU-3540).
--
-- State has no migration runner: an engineer applies this file by hand, before
-- deploying the gateway and state binaries that use it. Every statement is
-- idempotent, so re-running it is safe. init.sql carries the same definitions
-- for fresh installs.
--
-- voice_states, voice_guild_readiness and voice_shard_status are UNLOGGED like
-- the rest of the cache: an unclean Postgres restart empties them, which reads
-- as "not ready" until a fresh GUILD_CREATE baseline. voice_shard_losses is
-- LOGGED on purpose, so a recorded loss survives that same restart.

-- Older State databases predate voice_states in init.sql (staging had no such
-- table on 2026-11-10), so create the original shape first if it is missing.
CREATE UNLOGGED TABLE IF NOT EXISTS voice_states
(
	"guild_id" int8 NOT NULL,
	"user_id" int8 NOT NULL,
	"data" jsonb NOT NULL,
	PRIMARY KEY ("guild_id", "user_id")
);

ALTER TABLE voice_states ADD COLUMN IF NOT EXISTS channel_id int8 NOT NULL DEFAULT 0;
-- NULL means the account could not be classified; consumers must not treat it
-- as either a human or a bot.
ALTER TABLE voice_states ADD COLUMN IF NOT EXISTS is_bot bool;
ALTER TABLE voice_states ADD COLUMN IF NOT EXISTS changed_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE voice_states ADD COLUMN IF NOT EXISTS discord_seq int8 NOT NULL DEFAULT 0;

-- One row per guild whose voice presence was replaced from a complete
-- GUILD_CREATE on connection_id. A row from another connection is stale.
CREATE UNLOGGED TABLE IF NOT EXISTS voice_guild_readiness
(
	"guild_id" int8 NOT NULL,
	"shard_id" int4 NOT NULL,
	"connection_id" text NOT NULL,
	"baseline_at" timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY ("guild_id")
);

-- Per-shard heartbeat, refreshed every few seconds while the connection is
-- authenticated. ready is false while a loss is waiting to be recorded.
CREATE UNLOGGED TABLE IF NOT EXISTS voice_shard_status
(
	"shard_id" int4 NOT NULL,
	"connection_id" text NOT NULL,
	"ready" bool NOT NULL,
	"heartbeat_at" timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY ("shard_id")
);

-- Real voice observation losses: failed state writes or publications, and
-- identifies that could not resume. known_through is the last receipt time
-- the shard can vouch for. Rows are pruned after eight days.
CREATE TABLE IF NOT EXISTS voice_shard_losses
(
	"revision" int8 GENERATED ALWAYS AS IDENTITY,
	"shard_id" int4 NOT NULL,
	"reason" text NOT NULL,
	"known_through" timestamptz NOT NULL,
	"detected_at" timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY ("revision")
);

CREATE INDEX IF NOT EXISTS voice_shard_losses_shard_revision ON voice_shard_losses ("shard_id", "revision");
