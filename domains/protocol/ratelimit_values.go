package protocol

// Values RateLimitInfo.Status and RateLimitType are documented to take, from
// upstream's SDK types. The fields stay string and the sets are open: a newer
// CLI may send others. None is checked against a live run.
const (
	RateLimitAllowed        = "allowed"
	RateLimitAllowedWarning = "allowed_warning"
	RateLimitRejected       = "rejected"

	RateLimitTypeFiveHour       = "five_hour"
	RateLimitTypeSevenDay       = "seven_day"
	RateLimitTypeSevenDayOpus   = "seven_day_opus"
	RateLimitTypeSevenDaySonnet = "seven_day_sonnet"
	RateLimitTypeOverage        = "overage"
)
