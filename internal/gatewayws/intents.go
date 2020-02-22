package gatewayws

type Intent uint
type Intents []Intent

const (
	IntentGuilds Intent = iota
	IntentGuildMembers
	IntentGuildBans
	IntentGuildEmojis
	IntentGuildIntegrations
	IntentGuildWebhooks
	IntentGuildInvites
	IntentGuildVoiceStates
	IntentGuildPresences
	IntentGuildMessages
	IntentGuildMessageReactions
	IntentGuildMessageTyping
	IntentDirectMessages
	IntentDirectMessageReactions
	IntentDirectMessageTyping
)

func (i Intents) Collect() (n int) {
	for _, intent := range i {
		n += 1 << intent
	}

	return
}

var AllIntents = Intents{
	IntentGuilds,
	IntentGuildMembers,
	IntentGuildBans,
	IntentGuildEmojis,
	IntentGuildIntegrations,
	IntentGuildWebhooks,
	IntentGuildInvites,
	IntentGuildVoiceStates,
	IntentGuildPresences,
	IntentGuildMessages,
	IntentGuildMessageReactions,
	IntentGuildMessageTyping,
	IntentDirectMessages,
	IntentDirectMessageReactions,
	IntentDirectMessageTyping,
}

var DefaultIntents = Intents{
	IntentGuilds,
	IntentGuildMembers,
	IntentGuildBans,
	IntentGuildEmojis,
	IntentGuildIntegrations,
	IntentGuildWebhooks,
	IntentGuildInvites,
	IntentGuildVoiceStates,
	IntentGuildMessages,
	IntentGuildMessageReactions,
	IntentDirectMessages,
	IntentDirectMessageReactions,
}

var PresencesOnly = Intents{
	IntentGuildPresences,
}
