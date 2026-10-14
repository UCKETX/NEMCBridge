package encoding

// RoomTips ..
type RoomTips struct {
	LevelID            string
	GameType           uint8
	ConstantTestString string // Always is `Test`
	Vioce              int16
	ProtocolID         uint8
	// GameVersion is present in current NetEase clients after ProtocolID.
	GameVersion        string
}
