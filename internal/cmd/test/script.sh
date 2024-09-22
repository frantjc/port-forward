#!/bin/sh

iptables -t nat -A POSTROUTING --source 192.168.0.3 --destination 192.168.0.1 --jump SNAT --to-source 192.168.0.203

upnpc -a 192.168.0.203 80 80 TCP

upnpc -d 80 TCP

iptables -t nat -D POSTROUTING --source 192.168.0.3 --destination 192.168.0.1 --jump SNAT --to-source 192.168.0.203
