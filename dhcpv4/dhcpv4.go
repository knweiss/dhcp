package dhcpv4

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/insomniacslk/dhcp/iana"
)

// HeaderSize is the DHCPv4 header size in bytes.
const HeaderSize = 236

// MaxMessageSize is the maximum size in bytes that a DHCPv4 packet can hold.
const MaxMessageSize = 576

// DHCPv4 represents a DHCPv4 packet header and options. See the New* functions
// to build DHCPv4 packets.
type DHCPv4 struct {
	opcode         OpcodeType
	hwType         iana.HwTypeType
	hwAddrLen      uint8
	hopCount       uint8
	transactionID  uint32
	numSeconds     uint16
	flags          uint16
	clientIPAddr   net.IP
	yourIPAddr     net.IP
	serverIPAddr   net.IP
	gatewayIPAddr  net.IP
	clientHwAddr   [16]byte
	serverHostName [64]byte
	bootFileName   [128]byte
	options        []Option
}

// Modifier defines the signature for functions that can modify DHCPv4
// structures. This is used to simplify packet manipulation
type Modifier func(d *DHCPv4) *DHCPv4

// IPv4AddrsForInterface obtains the currently-configured, non-loopback IPv4
// addresses for iface.
func IPv4AddrsForInterface(iface *net.Interface) ([]net.IP, error) {
	if iface == nil {
		return nil, errors.New("IPv4AddrsForInterface: iface cannot be nil")
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	return GetExternalIPv4Addrs(addrs)
}

// GetExternalIPv4Addrs obtains the currently-configured, non-loopback IPv4
// addresses from `addrs` coming from a particular interface (e.g.
// net.Interface.Addrs).
func GetExternalIPv4Addrs(addrs []net.Addr) ([]net.IP, error) {
	var v4addrs []net.IP
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPAddr:
			ip = v.IP
		case *net.IPNet:
			ip = v.IP
		}

		if ip == nil || ip.IsLoopback() {
			continue
		}
		ip = ip.To4()
		if ip == nil {
			continue
		}
		v4addrs = append(v4addrs, ip)
	}
	return v4addrs, nil
}

// GenerateTransactionID generates a random 32-bits number suitable for use as
// TransactionID
func GenerateTransactionID() (*uint32, error) {
	b := make([]byte, 4)
	n, err := rand.Read(b)
	if n != 4 {
		return nil, errors.New("Invalid random sequence: smaller than 32 bits")
	}
	if err != nil {
		return nil, err
	}
	tid := binary.LittleEndian.Uint32(b)
	return &tid, nil
}

// New creates a new DHCPv4 structure and fill it up with default values. It
// won't be a valid DHCPv4 message so you will need to adjust its fields.
// See also NewDiscovery, NewOffer, NewRequest, NewAcknowledge, NewInform and
// NewRelease .
func New() (*DHCPv4, error) {
	tid, err := GenerateTransactionID()
	if err != nil {
		return nil, err
	}
	d := DHCPv4{
		opcode:        OpcodeBootRequest,
		hwType:        iana.HwTypeEthernet,
		hwAddrLen:     6,
		hopCount:      0,
		transactionID: *tid,
		numSeconds:    0,
		flags:         0,
		clientIPAddr:  net.IPv4zero,
		yourIPAddr:    net.IPv4zero,
		serverIPAddr:  net.IPv4zero,
		gatewayIPAddr: net.IPv4zero,
	}
	copy(d.clientHwAddr[:], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	copy(d.serverHostName[:], []byte{})
	copy(d.bootFileName[:], []byte{})
	options, err := OptionsFromBytes(MagicCookie)
	if err != nil {
		return nil, err
	}
	d.options = options
	// the End option has to be added explicitly
	d.AddOption(&OptionGeneric{OptionCode: OptionEnd})
	return &d, nil
}

// NewDiscoveryForInterface builds a new DHCPv4 Discovery message, with a default
// Ethernet HW type and the hardware address obtained from the specified
// interface.
func NewDiscoveryForInterface(ifname string) (*DHCPv4, error) {
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil, err
	}
	return NewDiscovery(iface.HardwareAddr)
}

// NewDiscovery builds a new DHCPv4 Discovery message, with a default Ethernet
// HW type and specified hardware address.
func NewDiscovery(hwaddr net.HardwareAddr) (*DHCPv4, error) {
	d, err := New()
	if err != nil {
		return nil, err
	}
	// get hw addr
	d.SetOpcode(OpcodeBootRequest)
	d.SetHwType(iana.HwTypeEthernet)
	d.SetHwAddrLen(uint8(len(hwaddr)))
	d.SetClientHwAddr(hwaddr)
	d.SetBroadcast()
	d.AddOption(&OptMessageType{MessageType: MessageTypeDiscover})
	d.AddOption(&OptParameterRequestList{
		RequestedOpts: []OptionCode{
			OptionSubnetMask,
			OptionRouter,
			OptionDomainName,
			OptionDomainNameServer,
		},
	})
	return d, nil
}

// NewInformForInterface builds a new DHCPv4 Informational message with default
// Ethernet HW type and the hardware address obtained from the specified
// interface.
func NewInformForInterface(ifname string, needsBroadcast bool) (*DHCPv4, error) {
	// get hw addr
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil, err
	}

	// Set Client IP as iface's currently-configured IP.
	localIPs, err := IPv4AddrsForInterface(iface)
	if err != nil || len(localIPs) == 0 {
		return nil, fmt.Errorf("could not get local IPs for iface %s", ifname)
	}
	pkt, err := NewInform(iface.HardwareAddr, localIPs[0])
	if err != nil {
		return nil, err
	}

	if needsBroadcast {
		pkt.SetBroadcast()
	} else {
		pkt.SetUnicast()
	}
	return pkt, nil
}

// NewInform builds a new DHCPv4 Informational message with default Ethernet HW
// type and specified hardware address. It does NOT put a DHCP End option at the
// end.
func NewInform(hwaddr net.HardwareAddr, localIP net.IP) (*DHCPv4, error) {
	d, err := New()
	if err != nil {
		return nil, err
	}

	d.SetOpcode(OpcodeBootRequest)
	d.SetHwType(iana.HwTypeEthernet)
	d.SetHwAddrLen(uint8(len(hwaddr)))
	d.SetClientHwAddr(hwaddr)
	d.SetClientIPAddr(localIP)
	d.AddOption(&OptMessageType{MessageType: MessageTypeInform})
	return d, nil
}

// NewRequestFromOffer builds a DHCPv4 request from an offer.
func NewRequestFromOffer(offer *DHCPv4, modifiers ...Modifier) (*DHCPv4, error) {
	d, err := New()
	if err != nil {
		return nil, err
	}
	d.SetOpcode(OpcodeBootRequest)
	d.SetHwType(offer.HwType())
	d.SetHwAddrLen(offer.HwAddrLen())
	hwaddr := offer.ClientHwAddr()
	d.SetClientHwAddr(hwaddr[:])
	d.SetTransactionID(offer.TransactionID())
	if offer.IsBroadcast() {
		d.SetBroadcast()
	} else {
		d.SetUnicast()
	}
	// find server IP address
	var serverIP []byte
	for _, opt := range offer.options {
		if opt.Code() == OptionServerIdentifier {
			serverIP = opt.(*OptServerIdentifier).ServerID
		}
	}
	if serverIP == nil {
		return nil, errors.New("Missing Server IP Address in DHCP Offer")
	}
	d.SetServerIPAddr(serverIP)
	d.AddOption(&OptMessageType{MessageType: MessageTypeRequest})
	d.AddOption(&OptRequestedIPAddress{RequestedAddr: offer.YourIPAddr()})
	d.AddOption(&OptServerIdentifier{ServerID: serverIP})
	for _, mod := range modifiers {
		d = mod(d)
	}
	return d, nil
}

// NewReplyFromRequest builds a DHCPv4 reply from a request.
func NewReplyFromRequest(request *DHCPv4, modifiers ...Modifier) (*DHCPv4, error) {
	reply, err := New()
	if err != nil {
		return nil, err
	}
	reply.SetOpcode(OpcodeBootReply)
	reply.SetHwType(request.HwType())
	reply.SetHwAddrLen(request.HwAddrLen())
	hwaddr := request.ClientHwAddr()
	reply.SetClientHwAddr(hwaddr[:])
	reply.SetTransactionID(request.TransactionID())
	reply.SetFlags(request.Flags())
	reply.SetGatewayIPAddr(request.GatewayIPAddr())
	for _, mod := range modifiers {
		reply = mod(reply)
	}
	return reply, nil
}

// FromBytes encodes the DHCPv4 packet into a sequence of bytes, and returns an
// error if the packet is not valid.
func FromBytes(data []byte) (*DHCPv4, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("Invalid DHCPv4 header: shorter than %v bytes", HeaderSize)
	}
	d := DHCPv4{
		opcode:        OpcodeType(data[0]),
		hwType:        iana.HwTypeType(data[1]),
		hwAddrLen:     data[2],
		hopCount:      data[3],
		transactionID: binary.BigEndian.Uint32(data[4:8]),
		numSeconds:    binary.BigEndian.Uint16(data[8:10]),
		flags:         binary.BigEndian.Uint16(data[10:12]),
		clientIPAddr:  net.IP(data[12:16]),
		yourIPAddr:    net.IP(data[16:20]),
		serverIPAddr:  net.IP(data[20:24]),
		gatewayIPAddr: net.IP(data[24:28]),
	}
	copy(d.clientHwAddr[:], data[28:44])
	copy(d.serverHostName[:], data[44:108])
	copy(d.bootFileName[:], data[108:236])
	options, err := OptionsFromBytes(data[236:])
	if err != nil {
		return nil, err
	}
	d.options = options
	return &d, nil
}

// Opcode returns the OpcodeType for the packet,
func (d *DHCPv4) Opcode() OpcodeType {
	return d.opcode
}

// OpcodeToString returns the mnemonic name for the packet's opcode.
func (d *DHCPv4) OpcodeToString() string {
	return d.opcode.String()
}

// SetOpcode sets a new opcode for the packet. It prints a warning if the opcode
// is unknown, but does not generate an error.
func (d *DHCPv4) SetOpcode(opcode OpcodeType) {
	if _, ok := OpcodeToString[opcode]; !ok {
		log.Printf("Warning: unknown DHCPv4 opcode: %v", opcode)
	}
	d.opcode = opcode
}

// HwType returns the hardware type as defined by IANA.
func (d *DHCPv4) HwType() iana.HwTypeType {
	return d.hwType
}

// HwTypeToString returns the mnemonic name for the hardware type, e.g.
// "Ethernet". If the type is unknown, it returns "Unknown".
func (d *DHCPv4) HwTypeToString() string {
	hwtype, ok := iana.HwTypeToString[d.hwType]
	if !ok {
		hwtype = "Invalid"
	}
	return hwtype
}

// SetHwType returns the hardware type as defined by IANA.
func (d *DHCPv4) SetHwType(hwType iana.HwTypeType) {
	if _, ok := iana.HwTypeToString[hwType]; !ok {
		log.Printf("Warning: Invalid DHCPv4 hwtype: %v", hwType)
	}
	d.hwType = hwType
}

// HwAddrLen returns the hardware address length. E.g. for Ethernet it would
// return 6.
func (d *DHCPv4) HwAddrLen() uint8 {
	return d.hwAddrLen
}

// SetHwAddrLen sets the hardware address length, limiting it to the maximum
// size 16 that the standard allows.
func (d *DHCPv4) SetHwAddrLen(hwAddrLen uint8) {
	if hwAddrLen > 16 {
		log.Printf("Warning: invalid HwAddrLen: %v > 16, using 16 instead", hwAddrLen)
		hwAddrLen = 16
	}
	d.hwAddrLen = hwAddrLen
}

// HopCount returns the hop count field.
func (d *DHCPv4) HopCount() uint8 {
	return d.hopCount
}

// SetHopCount sets the hop count value.
func (d *DHCPv4) SetHopCount(hopCount uint8) {
	d.hopCount = hopCount
}

// TransactionID returns the transaction ID as 32 bit unsigned integer.
func (d *DHCPv4) TransactionID() uint32 {
	return d.transactionID
}

// SetTransactionID sets the value for the transaction ID.
func (d *DHCPv4) SetTransactionID(transactionID uint32) {
	d.transactionID = transactionID
}

// NumSeconds returns the number of seconds.
func (d *DHCPv4) NumSeconds() uint16 {
	return d.numSeconds
}

// SetNumSeconds sets the seconds field.
func (d *DHCPv4) SetNumSeconds(numSeconds uint16) {
	d.numSeconds = numSeconds
}

// Flags returns the DHCP flags portion of the packet.
func (d *DHCPv4) Flags() uint16 {
	return d.flags
}

// SetFlags sets the flags field in the packet.
func (d *DHCPv4) SetFlags(flags uint16) {
	d.flags = flags
}

// FlagsToString returns a human-readable representation of the flags field.
func (d *DHCPv4) FlagsToString() string {
	flags := ""
	if d.IsBroadcast() {
		flags += "Broadcast"
	} else {
		flags += "Unicast"
	}
	if d.flags&0xfe != 0 {
		flags += " (reserved bits not zeroed)"
	}
	return flags
}

// IsBroadcast indicates whether the packet is a broadcast packet.
func (d *DHCPv4) IsBroadcast() bool {
	return d.flags&0x8000 == 0x8000
}

// SetBroadcast sets the packet to be a broadcast packet.
func (d *DHCPv4) SetBroadcast() {
	d.flags |= 0x8000
}

// IsUnicast indicates whether the packet is a unicast packet.
func (d *DHCPv4) IsUnicast() bool {
	return d.flags&0x8000 == 0
}

// SetUnicast sets the packet to be a unicast packet.
func (d *DHCPv4) SetUnicast() {
	d.flags &= ^uint16(0x8000)
}

// ClientIPAddr returns the client IP address.
func (d *DHCPv4) ClientIPAddr() net.IP {
	return d.clientIPAddr
}

// SetClientIPAddr sets the client IP address.
func (d *DHCPv4) SetClientIPAddr(clientIPAddr net.IP) {
	d.clientIPAddr = clientIPAddr
}

// YourIPAddr returns the "your IP address" field.
func (d *DHCPv4) YourIPAddr() net.IP {
	return d.yourIPAddr
}

// SetYourIPAddr sets the "your IP address" field.
func (d *DHCPv4) SetYourIPAddr(yourIPAddr net.IP) {
	d.yourIPAddr = yourIPAddr
}

// ServerIPAddr returns the server IP address.
func (d *DHCPv4) ServerIPAddr() net.IP {
	return d.serverIPAddr
}

// SetServerIPAddr sets the server IP address.
func (d *DHCPv4) SetServerIPAddr(serverIPAddr net.IP) {
	d.serverIPAddr = serverIPAddr
}

// GatewayIPAddr returns the gateway IP address.
func (d *DHCPv4) GatewayIPAddr() net.IP {
	return d.gatewayIPAddr
}

// SetGatewayIPAddr sets the gateway IP address.
func (d *DHCPv4) SetGatewayIPAddr(gatewayIPAddr net.IP) {
	d.gatewayIPAddr = gatewayIPAddr
}

// ClientHwAddr returns the client hardware (MAC) address.
func (d *DHCPv4) ClientHwAddr() [16]byte {
	return d.clientHwAddr
}

// ClientHwAddrToString converts the hardware address field to a string.
func (d *DHCPv4) ClientHwAddrToString() string {
	var ret []string
	for _, b := range d.clientHwAddr[:d.hwAddrLen] {
		ret = append(ret, fmt.Sprintf("%02x", b))
	}
	return strings.Join(ret, ":")
}

// SetClientHwAddr sets the client hardware address.
func (d *DHCPv4) SetClientHwAddr(clientHwAddr []byte) {
	if len(clientHwAddr) > 16 {
		log.Printf("Warning: too long HW Address (%d bytes), truncating to 16 bytes", len(clientHwAddr))
		clientHwAddr = clientHwAddr[:16]
	}
	copy(d.clientHwAddr[:len(clientHwAddr)], clientHwAddr)
	// pad the remaining bytes, if any
	for i := len(clientHwAddr); i < 16; i++ {
		d.clientHwAddr[i] = 0
	}
}

// ServerHostName returns the server host name as a sequence of bytes.
func (d *DHCPv4) ServerHostName() [64]byte {
	return d.serverHostName
}

// ServerHostNameToString returns the server host name as a string, after
// trimming the null bytes at the end.
func (d *DHCPv4) ServerHostNameToString() string {
	return strings.TrimRight(string(d.serverHostName[:]), "\x00")
}

// SetServerHostName replaces the server host name, from a sequence of bytes,
// truncating it to the maximum length of 64.
func (d *DHCPv4) SetServerHostName(serverHostName []byte) {
	if len(serverHostName) > 64 {
		serverHostName = serverHostName[:64]
	} else if len(serverHostName) < 64 {
		for i := len(serverHostName) - 1; i < 64; i++ {
			serverHostName = append(serverHostName, 0)
		}
	}
	// need an array, not a slice, so let's copy it
	var newServerHostName [64]byte
	copy(newServerHostName[:], serverHostName)
	d.serverHostName = newServerHostName
}

// BootFileName returns the boot file name as a sequence of bytes.
func (d *DHCPv4) BootFileName() [128]byte {
	return d.bootFileName
}

// BootFileNameToString returns the boot file name as a string, after trimming
// the null bytes at the end.
func (d *DHCPv4) BootFileNameToString() string {
	return strings.TrimRight(string(d.bootFileName[:]), "\x00")
}

// SetBootFileName replaces the boot file name, from a sequence of bytes,
// truncating it to the maximum length oh 128.
func (d *DHCPv4) SetBootFileName(bootFileName []byte) {
	if len(bootFileName) > 128 {
		bootFileName = bootFileName[:128]
	} else if len(bootFileName) < 128 {
		for i := len(bootFileName) - 1; i < 128; i++ {
			bootFileName = append(bootFileName, 0)
		}
	}
	// need an array, not a slice, so let's copy it
	var newBootFileName [128]byte
	copy(newBootFileName[:], bootFileName)
	d.bootFileName = newBootFileName
}

// Options returns the DHCPv4 options defined for the packet.
func (d *DHCPv4) Options() []Option {
	return d.options
}

// GetOption will attempt to get all options that match a DHCPv4 option
// from its OptionCode.  If the option was not found it will return an
// empty list.
func (d *DHCPv4) GetOption(code OptionCode) []Option {
	opts := []Option{}
	for _, opt := range d.Options() {
		if opt.Code() == code {
			opts = append(opts, opt)
		}
	}
	return opts
}

// GetOneOption will attempt to get an  option that match a Option code.
// If there are multiple options with the same OptionCode it will only return
// the first one found.  If no matching option is found nil will be returned.
func (d *DHCPv4) GetOneOption(code OptionCode) Option {
	for _, opt := range d.Options() {
		if opt.Code() == code {
			return opt
		}
	}
	return nil
}

// StrippedOptions works like Options, but it does not return anything after the
// End option.
func (d *DHCPv4) StrippedOptions() []Option {
	// differently from Options() this function strips away anything coming
	// after the End option (normally just Pad options).
	strippedOptions := []Option{}
	for _, opt := range d.options {
		strippedOptions = append(strippedOptions, opt)
		if opt.Code() == OptionEnd {
			break
		}
	}
	return strippedOptions
}

// SetOptions replaces the current options with the provided ones.
func (d *DHCPv4) SetOptions(options []Option) {
	d.options = options
}

// AddOption appends an option to the existing ones. If the last option is an
// OptionEnd, it will be inserted before that. It does not deal with End
// options that appead before the end, like in malformed packets.
func (d *DHCPv4) AddOption(option Option) {
	if len(d.options) == 0 || d.options[len(d.options)-1].Code() != OptionEnd {
		d.options = append(d.options, option)
	} else {
		end := d.options[len(d.options)-1]
		d.options[len(d.options)-1] = option
		d.options = append(d.options, end)
	}
}

// MessageType returns the message type, trying to extract it from the
// OptMessageType option. It returns nil if the message type cannot be extracted
func (d *DHCPv4) MessageType() *MessageType {
	opt := d.GetOneOption(OptionDHCPMessageType)
	if opt == nil {
		return nil
	}
	return &(opt.(*OptMessageType).MessageType)
}

func (d *DHCPv4) String() string {
	return fmt.Sprintf("DHCPv4(opcode=%v hwtype=%v hwaddr=%v)",
		d.OpcodeToString(), d.HwTypeToString(), d.ClientHwAddr())
}

// Summary prints detailed information about the packet.
func (d *DHCPv4) Summary() string {
	ret := fmt.Sprintf(
		"DHCPv4\n"+
			"  opcode=%v\n"+
			"  hwtype=%v\n"+
			"  hwaddrlen=%v\n"+
			"  hopcount=%v\n"+
			"  transactionid=0x%08x\n"+
			"  numseconds=%v\n"+
			"  flags=%v (0x%02x)\n"+
			"  clientipaddr=%v\n"+
			"  youripaddr=%v\n"+
			"  serveripaddr=%v\n"+
			"  gatewayipaddr=%v\n"+
			"  clienthwaddr=%v\n"+
			"  serverhostname=%v\n"+
			"  bootfilename=%v\n",
		d.OpcodeToString(),
		d.HwTypeToString(),
		d.HwAddrLen(),
		d.HopCount(),
		d.TransactionID(),
		d.NumSeconds(),
		d.FlagsToString(),
		d.Flags(),
		d.ClientIPAddr(),
		d.YourIPAddr(),
		d.ServerIPAddr(),
		d.GatewayIPAddr(),
		d.ClientHwAddrToString(),
		d.ServerHostNameToString(),
		d.BootFileNameToString(),
	)
	ret += "  options=\n"
	for _, opt := range d.options {
		optString := opt.String()
		// If this option has sub structures, offset them accordingly.
		if strings.Contains(optString, "\n") {
			optString = strings.Replace(optString, "\n  ", "\n      ", -1)
		}
		ret += fmt.Sprintf("    %v\n", optString)
		if opt.Code() == OptionEnd {
			break
		}
	}
	return ret
}

// ValidateOptions runs sanity checks on the DHCPv4 packet and prints a number
// of warnings if something is incorrect.
func (d *DHCPv4) ValidateOptions() {
	// TODO find duplicate options
	foundOptionEnd := false
	for _, opt := range d.options {
		if foundOptionEnd {
			if opt.Code() == OptionEnd {
				log.Print("Warning: found duplicate End option")
			}
			if opt.Code() != OptionEnd && opt.Code() != OptionPad {
				log.Printf("Warning: found option %v (%v) after End option", opt.Code(), opt.Code().String())
			}
		}
		if opt.Code() == OptionEnd {
			foundOptionEnd = true
		}
	}
	if !foundOptionEnd {
		log.Print("Warning: no End option found")
	}
}

// IsOptionRequested returns true if that option is within the requested
// options of the DHCPv4 message.
func (d *DHCPv4) IsOptionRequested(requested OptionCode) bool {
	for _, optprl := range d.GetOption(OptionParameterRequestList) {
		for _, o := range optprl.(*OptParameterRequestList).RequestedOpts {
			if o == requested {
				return true
			}
		}
	}
	return false
}

// ToBytes encodes a DHCPv4 structure into a sequence of bytes in its wire
// format.
func (d *DHCPv4) ToBytes() []byte {
	// This won't check if the End option is present, you've been warned
	var ret []byte
	u32 := make([]byte, 4)
	u16 := make([]byte, 2)

	ret = append(ret, byte(d.opcode))
	ret = append(ret, byte(d.hwType))
	ret = append(ret, byte(d.hwAddrLen))
	ret = append(ret, byte(d.hopCount))
	binary.BigEndian.PutUint32(u32, d.transactionID)
	ret = append(ret, u32...)
	binary.BigEndian.PutUint16(u16, d.numSeconds)
	ret = append(ret, u16...)
	binary.BigEndian.PutUint16(u16, d.flags)
	ret = append(ret, u16...)
	ret = append(ret, d.clientIPAddr.To4()...)
	ret = append(ret, d.yourIPAddr.To4()...)
	ret = append(ret, d.serverIPAddr.To4()...)
	ret = append(ret, d.gatewayIPAddr.To4()...)
	ret = append(ret, d.clientHwAddr[:16]...)
	ret = append(ret, d.serverHostName[:64]...)
	ret = append(ret, d.bootFileName[:128]...)

	d.ValidateOptions() // print warnings about broken options, if any
	ret = append(ret, MagicCookie...)
	for _, opt := range d.options {
		ret = append(ret, opt.ToBytes()...)
	}
	return ret
}

// OptionGetter is a interface that knows how to retrieve an option from a
// structure of options given an OptionCode.
type OptionGetter interface {
	GetOption(OptionCode) []Option
	GetOneOption(OptionCode) Option
}

// HasOption checks whether the OptionGetter `o` has the given `opcode` Option.
func HasOption(o OptionGetter, opcode OptionCode) bool {
	return o.GetOneOption(opcode) != nil
}
