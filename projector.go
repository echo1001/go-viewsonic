package projector

import (
	"time"

	"github.com/tarm/serial"
)

type Response struct {
	Size uint16
	Data []byte
}

type CommandType byte

const COMMAND_EXCEPTION CommandType = 0
const COMMAND_ACK CommandType = 3
const COMMAND_RESPONSE CommandType = 5
const COMMAND_WRITE CommandType = 6
const COMMAND_READ CommandType = 7

type Packet struct {
	Command CommandType
	Data    []byte
}

func (p *Packet) DataLength() []byte {
	var lenBytes = []byte{0, 0}
	var numBytes = len(p.Data)
	lenBytes[0] = byte((numBytes & 0x00FF))
	lenBytes[1] = byte((numBytes & 0xFF00) >> 8)

	return lenBytes
}

func (p *Packet) Checksum() byte {
	var sum byte = 0
	var lenBytes = p.DataLength()
	sum += lenBytes[0] + lenBytes[1]
	for _, b := range p.Data {
		sum += b
	}
	return sum + 0x14
}

func (p *Packet) Build() []byte {
	var bytes []byte = []byte{byte(p.Command)}

	bytes = append(bytes, []byte{0x14, 0x00}...)
	bytes = append(bytes, p.DataLength()...)
	bytes = append(bytes, p.Data...)
	bytes = append(bytes, p.Checksum())

	return bytes
}

type Projector struct {
	Port *serial.Port
}

type ProjectorError string

func (e ProjectorError) Error() string {
	return string(e)
}

func (p *Projector) Open(portName string) error {
	if p.Port != nil {
		p.Port.Close()
		p.Port = nil
	}
	opt := &serial.Config{Baud: 115200, Name: portName, Size: 8, StopBits: 1, ReadTimeout: time.Millisecond * 100, Parity: serial.ParityNone}
	var err error
	p.Port, err = serial.OpenPort(opt)
	if err != nil {
		p.Port = nil
	}
	return err
}

func (p *Projector) Close() {
	if p.Port != nil {
		p.Port.Close()
		p.Port = nil
	}
}

// Response Ref pg 74: http://www.projectorcentral.com/pdf/projector_manual_7407.pdf

func (p *Projector) ReadResponse() (*Packet, error) {
	if p.Port == nil {
		return nil, ProjectorError("Port not open")
	}
	var preamble []byte
	count := 0
	var err error
	for {
		buffer := make([]byte, 5-count)
		n, err := p.Port.Read(buffer)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		preamble = append(preamble, buffer[:n]...)
		count += n
		if count == 5 {
			break
		}
	}

	var packet = Packet{}
	packet.Command = CommandType(preamble[0])
	packet.Data = []byte{}
	dataLength := int(preamble[3]) + (int(preamble[4]) << 8)
	count = 0
	if dataLength > 0 {
		for {
			buffer := make([]byte, dataLength-count)
			n, err := p.Port.Read(buffer)
			if err != nil {
				return nil, err
			}
			if n == 0 {
				break
			}
			packet.Data = append(packet.Data, buffer[:n]...)
			count += n
			if count == dataLength {
				break
			}
		}
	}
	chkSum := make([]byte, 1)
	_, err = p.Port.Read(chkSum)
	if err != nil {
		return nil, err
	}

	if packet.Checksum() != chkSum[0] {
		return nil, ProjectorError("Checksum failed")
	}

	return &packet, nil
}

func (p *Projector) Write(packet Packet) error {
	var err error

	if p.Port == nil {
		return ProjectorError("Port not open")
	}
	_, err = p.Port.Write(packet.Build())
	if err != nil {
		return err
	}

	return nil
}

func (p *Projector) WriteAndRead(packet Packet) (*Packet, error) {
	if p.Port == nil {
		return nil, ProjectorError("Port not open")
	}

	err := p.Port.Flush()
	if err != nil {
		return nil, err
	}

	err = p.Write(packet)
	if err != nil {
		return nil, err
	}

	rPacket, err := p.ReadResponse()
	if err != nil {
		return nil, err
	}

	if rPacket.Command == COMMAND_EXCEPTION {
		return nil, ProjectorError("Projector returned exception")
	}
	return rPacket, err
}

func getBool(bytes byte) bool {
	return bytes > 0
}

func getUint32(bytes []byte) uint32 {
	var ret = uint32(0)
	ret += uint32(bytes[0])
	ret += uint32(bytes[1]) << 8
	ret += uint32(bytes[2]) << 16
	ret += uint32(bytes[3]) << 24
	return ret
}

// Command Table Ref pg. 66: https://www.viewsoniceurope.com/asset-files/files/user_guide/pjd7820hd/28077.pdf

func (p *Projector) PowerState() (bool, error) {
	packet := Packet{Command: COMMAND_READ, Data: []byte{0x34, 0x00, 0x00, 0x11, 0x00}}

	rPacket, err := p.WriteAndRead(packet)
	if err != nil {
		return false, err
	}
	return getBool(rPacket.Data[2]), nil
}

func (p *Projector) PowerOff() error {
	packet := Packet{Command: COMMAND_WRITE, Data: []byte{0x34, 0x11, 0x01, 0x00}}

	_, err := p.WriteAndRead(packet)
	if err != nil {
		return err
	}
	return nil
}

func (p *Projector) PowerOn() error {
	packet := Packet{Command: COMMAND_WRITE, Data: []byte{0x34, 0x11, 0x00, 0x00}}

	_, err := p.WriteAndRead(packet)
	if err != nil {
		return err
	}
	return nil
}

func (p *Projector) LampHours() (uint32, error) {
	packet := Packet{Command: COMMAND_READ, Data: []byte{0x34, 0x00, 0x00, 0x15, 0x01}}

	rPacket, err := p.WriteAndRead(packet)
	if err != nil {
		return 0, err
	}
	return getUint32(rPacket.Data[2:]), nil
}
