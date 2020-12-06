# -*- coding: utf-8 -*-

import sys
import json

def avg(latencies):
    if len(latencies) == 0:
        return 0
    sum = 0
    for latency in latencies:
        sum += int(latency)
    return sum/len(latencies)

if __name__ == '__main__':
    if len(sys.argv) != 2:
        print('Usage: %s filename' % (sys.argv[0]))
        sys.exit(1)

    avg_latency = []

    with open(sys.argv[1], 'r') as f:
        s = f.readline()
        while s:
            try:
                j = json.loads(s)
                # print(j['AverageLatency'])
                avg_latency.append(j['AverageLatency'])
            except:
                pass
            s = f.readline()

    avg_avg_latency = avg(avg_latency)
    print(avg_avg_latency)
    '''for latency in avg_latency:
        if latency > avg_avg_latency:
            print(latency)'''
