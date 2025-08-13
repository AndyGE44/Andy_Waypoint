import os
import time
from datetime import datetime

def flush_and_sync(f):
    f.flush()
    os.fsync(f.fileno())

def report_out_files():
    """
    Every 5 seconds: append `*.out` files in cwd to `report.log` with timestamp.
    """
    with open("report.log", "a") as log:
        ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        files = [f for f in os.listdir(".") if f.endswith(".out")]
        files.sort()
        log.write(f"{ts} {' '.join(files)}\n")
        flush_and_sync(log)

def write_counter_file(counter):
    """
    Every 10 seconds: write timestamp & counter to a new `HHMMSS-counter.out` file.
    """
    ts = datetime.now()
    filename = f"{ts.strftime('%H%M%S')}-{counter}.out"
    with open(filename, "w") as f:
        f.write(f"{ts.strftime('%Y-%m-%d %H:%M:%S')} [{counter}]\n")
        flush_and_sync(f)

def main():
    counter = 0
    start = time.time()
    next_report = start
    next_write = start
    while counter < 100:
        now = time.time()
        if now >= next_report:
            report_out_files()
            next_report += 5
        if now >= next_write:
            write_counter_file(counter)
            counter += 1
            next_write += 10
        time.sleep(1)

if __name__ == "__main__":
    main()
