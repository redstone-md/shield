package events

func (a *admin) logConfig() adminLogConfig {
	if a == nil {
		return adminLogConfig{}
	}
	return adminLogConfig{
		SuperUsers:             a.superUsers,
		PrimaryChatID:          a.firstChatID(),
		AdminChatID:            a.adminChatID,
		TrainingMode:           a.trainingMode,
		SoftBan:                a.softBan,
		Dry:                    a.dry,
		WarnMessage:            a.warnMsg,
		AggressiveCleanup:      a.aggressiveCleanup,
		AggressiveCleanupLimit: a.aggressiveCleanupLimit,
	}
}

type adminLogConfig struct {
	SuperUsers             SuperUsers
	PrimaryChatID          int64
	AdminChatID            int64
	TrainingMode           bool
	SoftBan                bool
	Dry                    bool
	WarnMessage            string
	AggressiveCleanup      bool
	AggressiveCleanupLimit int
}
