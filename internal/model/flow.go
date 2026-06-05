// Package model defines the goflow2 flow record structure.
package model

// Flow represents a single goflow2 IPFIX/NetFlow/sFlow export record as emitted
// by goflow2's JSON transport (one JSON object per line, NDJSON).
//
// Only the fields used for indexed querying are typed explicitly. The complete
// original record is preserved separately as raw JSON so the API can return the
// source object verbatim, regardless of any extra fields goflow2 emits.
type Flow struct {
	Type           string `json:"type"`
	TimeReceivedNs int64  `json:"time_received_ns"`
	TimeFlowStart  int64  `json:"time_flow_start_ns"`
	TimeFlowEnd    int64  `json:"time_flow_end_ns"`
	SequenceNum    int64  `json:"sequence_num"`
	SamplerAddress string `json:"sampler_address"`
	Bytes          int64  `json:"bytes"`
	Packets        int64  `json:"packets"`
	SrcAddr        string `json:"src_addr"`
	DstAddr        string `json:"dst_addr"`
	Etype          string `json:"etype"`
	Proto          string `json:"proto"`
	SrcPort        int64  `json:"src_port"`
	DstPort        int64  `json:"dst_port"`
}
