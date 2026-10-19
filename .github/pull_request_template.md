## Summary

<!-- What changed and why. -->

## Deploy note

<!-- Does a correct deploy of this change require someone to do something that
     the image bump and the merged code do NOT do by themselves?

       · deploy ordering between gateway and state, or with a consumer
         ("state before gateway", "gateway before worker reads the new event")
       · a Postgres schema change or backfill to run by hand
       · an env var (MULTI_REDIS, INTENTS, …) / secret that must change first
       · a shard restart or force-identify after rollout
       · a breaking event or state-API contract needing a coordinated consumer release

     If yes: replace this comment with the action and, in one line, what breaks
     if it is skipped. On merge to master, CI appends it to Pending Deploy
     Actions in Linear and the deployer drains it.

     If no: DELETE THIS WHOLE SECTION. Most PRs have no deploy note. -->
