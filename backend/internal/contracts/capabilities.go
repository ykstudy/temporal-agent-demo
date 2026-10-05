package contracts

// DeliveryContract lists published versions containing the full 1.3 delivery
// evidence vocabulary. Unknown future strings never receive implied support.
func DeliveryContract(version string) bool { return version == "1.3.0" || version == "1.4.0" }
