iptables -t nat -A POSTROUTING --source 192.168.0.3 --destination 192.168.0.1 --jump SNAT --to-source 192.168.0.222
upnpc -a 192.168.0.222 80 80 TCP
iptables -t nat -D POSTROUTING -s 192.168.0.3 -d 192.168.0.1 -j SNAT --to-source 192.168.0.222
