import os
import time
import threading
import argparse
from datetime import datetime

def flush_and_sync(f):
    f.flush()
    os.fsync(f.fileno())

def report_out_files():
    """
    Append .out files in cwd to log_path with timestamp.
    """
    with open("/tmp/report.log", "a") as log:
        ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        files = [f for f in os.listdir(".") if f.endswith(".out")]
        log.write(f"{ts} {' '.join(files)}\n")
        flush_and_sync(log)

def write_counter_file(counter):
    """
    Write timestamp & counter to a new .out file.
    """
    ts = datetime.now()
    filename = f"{ts.strftime('%H%M%S')}-{counter}.out"
    with open(filename, "w") as f:
        f.write(f"{ts.strftime('%Y-%m-%d %H:%M:%S')} [{counter}]\n")
        flush_and_sync(f)

def single_threaded():
    """
    Single-threaded version with precise scheduling.
    """
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

def write_counter_worker():
    """
    Worker thread for multithreaded mode.
    """
    counter = 0
    while counter < 100:
        write_counter_file(counter)
        counter += 1
        time.sleep(10)

def multi_threaded():
    """
    Multi-threaded mode: main thread = reporting, worker thread = writing.
    """
    worker = threading.Thread(target=write_counter_worker, daemon=True)
    worker.start()
    while worker.is_alive():
        report_out_files()
        time.sleep(5)

def main():
    parser = argparse.ArgumentParser(description="File reporting and writing program")
    parser.add_argument("--mode", choices=["single", "multi"], default="single",
                        help="Choose 'single' for single-threaded or 'multi' for multi-threaded mode")
    args = parser.parse_args()

    if args.mode == "single":
        single_threaded()
    else:
        multi_threaded()

if __name__ == "__main__":
    main()
