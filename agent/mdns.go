package agent

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

const MdnsTimeout = 5 * time.Second

var multicastAddress = net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// mDnsResolve resolves a .local (mDNS) hostname to an IPv4 address. It sends a raw
// DNS A-record query to the mDNS multicast address (224.0.0.251:5353) and
// returns the first matching IPv4 address.
func mDnsResolve(host string) (string, error) {
	// Build an A-record DNS query using the standard message library
	packed, err := new(dnsmessage.Message{
		Header: dnsmessage.Header{ID: 0, Response: false},
		Questions: []dnsmessage.Question{{
			Name:  dnsmessage.MustNewName(host + "."),
			Type:  dnsmessage.TypeA,
			Class: dnsmessage.ClassINET,
		}},
	}).Pack()
	if err != nil {
		return "", fmt.Errorf("mDNS: failed to pack query: %w", err)
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		return "", fmt.Errorf("mDNS: failed to create UDP socket: %w", err)
	}
	defer conn.Close()
	if _, err := conn.WriteTo(packed, &multicastAddress); err != nil {
		return "", fmt.Errorf("mDNS: failed to send query: %w", err)
	} else if err := conn.SetReadDeadline(time.Now().Add(MdnsTimeout)); err != nil {
		return "", fmt.Errorf("mDNS: failed to set deadline: %w", err)
	}
	for buffer := [1500]byte{}; ; {
		count, _, err := conn.ReadFromUDP(buffer[:])
		if err != nil {
			break
		}
		var message dnsmessage.Message
		if err := message.Unpack(buffer[:count]); err != nil {
			continue
		}
		for _, answer := range message.Answers {
			if answer.Header.Name.String() == host+"." && answer.Header.Type == dnsmessage.TypeA {
				if aBody, ok := answer.Body.(*dnsmessage.AResource); ok {
					return net.IP(aBody.A[:]).String(), nil
				}
			}
		}
	}

	return "", fmt.Errorf("mDNS: no A record found for %s", host)
}
