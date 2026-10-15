# Native room protocol 860 permission samples

Captured on 2026-10-15 from a user-created NetEase local multiplayer room,
with a member bot joining through authenticated room signaling. The game
client version in this conversation is 3.9.41.300665 (core 1.21.120).

- `native-860-member-abilities.hex`: complete UpdateAbilities packet (187).
  Player permission 1, command permission 0, one base layer, mask 0xfffff,
  values 0xd3f, speeds 0.05/1/0.1. The trailing fields are Uint32(0), Int64(-1).
  The room owner's operator packet also used the same trailing fields.
- `native-860-add-player-tail.hex`: only the final 16 bytes of AddPlayer (12):
  empty string, empty string, false, true, Uint32(0), Int64(-1).

These are observed wire values. The precise meaning of the proprietary
trailing fields is not established. They do not contain account tokens,
player names, account UUIDs, or chat messages. The ability sample's entity ID
is only a world actor ID. Matching these samples tests wire compatibility;
it does not prove that the NetEase UI shows the intended permission.
