package discord

type Encoding interface {
	Name() string

	DecodeHello(buf []byte) (int, string, error)
	DecodeReady(buf []byte) (version int, sessionID string, _ error)

	DecodeT(buf []byte) (*Event, error)

	DecodeChannel(buf []byte) (*Channel, error)
	DecodeVoiceState(buf []byte) (*VoiceState, error)

	DecodeGuildCreate(buf []byte) (*GuildCreate, error)
	DecodeGuildBan(buf []byte) (*GuildBan, error)

	DecodeMemberChunk(buf []byte) (*MemberChunk, error)
	DecodeMember(buf []byte) (*Member, error)

	DecodePresence(buf []byte) (*Presence, error)
	DecodePlayedPresence(buf []byte) (*PlayedPresence, error)

	DecodeMessage(buf []byte) (*Message, error)
	DecodeMessageReaction(buf []byte) (*MessageReaction, error)
	DecodeMessageReactionRemoveAll(buf []byte) (*MessageReactionRemoveAll, error)

	DecodeRole(buf []byte) (*Role, error)
	DecodeRoleDelete(buf []byte) (*RoleDelete, error)

	Write(obj interface{}) ([]byte, error)
}
