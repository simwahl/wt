##
# Work Time (like "time to work" and actually "timing" work :D)
##

from datetime import datetime as dt, timedelta
from enum import StrEnum
from typing import List
import sys
import os
import shutil
import re
import json

# Keep updated with .gitignore !
OUTPUT_FOLDER = ".out"
OUTPUT_FILE_NAME = "wt.json"
DEBUG_LOG_NAME = "debug-log"
INFO_LOG_NAME = "info-log"
DEBUG_LOG_PATH = f"{OUTPUT_FOLDER}/{DEBUG_LOG_NAME}"
INFO_LOG_PATH = f"{OUTPUT_FOLDER}/{INFO_LOG_NAME}"
OUTPUT_FILE_PATH = f"{OUTPUT_FOLDER}/{OUTPUT_FILE_NAME}"

DT_FORMAT = "%Y-%m-%d %H:%M:%S"
TIME_ONLY_FORMAT = "%H:%M:%S"


class Status(StrEnum):
    Stopped = "stopped"
    Paused = "paused"
    Running = "running"


class Mode(StrEnum):
    Silent = "silent"
    Normal = "normal"
    Verbose = "verbose"


class Timer():
    def __init__(
            self,
            status=Status.Stopped,
            start="", stop="", pausedTime=0,
            mode=Mode.Silent,
            timeline=None):
        self.status: Status = status
        self.start_datetime_str: str = start
        self.stop_datetime_str: str = stop
        self.paused_minutes: int = pausedTime
        self.mode: Mode = mode
        # Timeline entries: {"type": "work"|"break", "start": timestamp, "stop": timestamp}
        self.timeline: List[dict] = timeline if timeline is not None else []

    def __str__(self):
        return (
            f"status = {self.status}\n"
            f"start_datetime_sr = {self.start_datetime_str}\n"
            f"stop_datetime_str = {self.stop_datetime_str}\n"
            f"paused_minutes = {self.paused_minutes}\n"
            f"mode = {self.mode}\n"
            f"timeline = {self.timeline}\n"
        )
    
    def completed_minutes(self) -> int:
        """Calculate total completed minutes from work cycles in timeline."""
        total = 0
        for entry in self.timeline:
            if entry["type"] == "work":
                start_dt = dt.strptime(entry["start"], DT_FORMAT)
                stop_dt = dt.strptime(entry["stop"], DT_FORMAT)
                total += delta_minutes(start_dt, stop_dt)
        return total


def main():
    args = sys.argv[1:]
    if len(args) == 0:
        check()
        return

    match args[0]:
        case "start":
            start_time = None if len(args) < 2 else args[1]
            start(start_time)
        case "stop":
            stop()
        case "pause":
            pause()
        case "check":
            check()
        case "add":
            if len(args) != 2:
                print("Incorrect amount of arguments.")
                return
            add(args[1])
        case "sub":
            if len(args) != 2:
                print("Incorrect amount of arguments.")
                return
            sub(args[1])
        case "log":
            log_type = None if len(args) < 2 else args[1]
            history(log_type)
        case "next":
            next_timer()
        case "reset":
            reset()
        case "restart":
            start_time = None if len(args) < 2 else args[1]
            restart(start_time)
        case "new":
            new()
        case "remove":
            remove()
        case "status":
            status()
        case "mode":
            if len(args) < 2:
                timer = load()
                print(timer.mode)
                return
            mode_select(args[1])
        case "help":
            print_help()
        case "debug":
            debug()
        case _:
            print("Invalid command.")


def start(start_time: str = None):
    if start_time:
        validate_timestring_or_quit(start_time)

    if not os.path.exists(output_file_path()):
        reset()

    timer = load()
    message = ""
    match timer.status:
        case Status.Running:
            print("Already running.")
            return
        case Status.Paused:
            message = "Resuming timer."
        case Status.Stopped:
            message = "Starting timer."

    break_time_str = ""
    if timer.stop_datetime_str != "":
        # Create a break timeline entry
        break_start = timer.stop_datetime_str
        break_stop = dt.now().strftime(DT_FORMAT)
        timer.timeline.append({
            "type": "break",
            "start": break_start,
            "stop": break_stop
        })
        break_mins = delta_minutes(
            dt.strptime(break_start, DT_FORMAT), dt.strptime(break_stop, DT_FORMAT))
        break_time_str = mintues_to_hour_minute_str(break_mins)

    timer.stop_datetime_str = ""
    now = dt.now().strftime(DT_FORMAT)
    timer.start_datetime_str = now
    prev_status = timer.status
    timer.status = Status.Running

    start_time_log = f" {start_time}" if start_time != None else ""
    log_debug(f"wt start{start_time_log}")

    save(timer)
    print_message_if_not_silent(timer, message)
    print_check_if_verbose(timer)

    if start_time != None:
        if prev_status != Status.Stopped:
            print("Can only set start time if stopped")
            return
        else:
            # Backdate the start time by the specified minutes
            minutes = string_time_to_minutes(start_time)
            timer = load()
            start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
            new_start_dt = start_dt - timedelta(minutes=minutes)
            timer.start_datetime_str = new_start_dt.strftime(DT_FORMAT)
            save(timer)


def stop():
    timer = load()
    cycle_minutes = 0
    cycle_start_str = ""
    match timer.status:
        case Status.Stopped:
            print("Timer already stopped.")
        case Status.Running | Status.Paused:
            now = dt.now()
            stop_time_str = now.strftime(DT_FORMAT)
            
            # Create work timeline entry
            # For paused timers, calculate the start based on accumulated paused time
            if timer.status == Status.Paused:
                work_start = (now - timedelta(minutes=timer.paused_minutes)).strftime(DT_FORMAT)
                cycle_minutes = timer.paused_minutes
            else:
                work_start = timer.start_datetime_str
                cycle_minutes = delta_minutes(dt.strptime(timer.start_datetime_str, DT_FORMAT), now)
                cycle_minutes += timer.paused_minutes
                
            timer.timeline.append({
                "type": "work",
                "start": work_start,
                "stop": stop_time_str
            })
            
            timer.stop_datetime_str = stop_time_str
            timer.start_datetime_str = ""
            timer.paused_minutes = 0
            timer.status = Status.Stopped

            log_debug("wt stop")
            
            cycle_str = mintues_to_hour_minute_str(cycle_minutes)
            total_str = mintues_to_hour_minute_str(timer.completed_minutes())
            
            if work_start:
                start_time_only = dt.strptime(work_start, DT_FORMAT).strftime(TIME_ONLY_FORMAT)
                end_time_only = now.strftime(TIME_ONLY_FORMAT)
                log_info(f"[{start_time_only} => {end_time_only}] {'Completed cycle:':<22} {cycle_str} ({total_str})")
            else:
                log_info(f"{'Completed cycle:':<22} {cycle_str} ({total_str})")
            
            save(timer)
            print_message_if_not_silent(timer, "Timer stopped.")
            print_check_if_verbose(timer)
        case _:
            print(f"Unhandled status: {timer.status}")


def pause():
    timer = load()
    match timer.status:
        case Status.Paused:
            print("Timer already paused.")
        case Status.Stopped:
            print("Cannot pause stopped timer.")
        case Status.Running:
            timer.paused_minutes = calculate_current_minutes(timer)
            timer.start_datetime_str = ""
            timer.status = Status.Paused

            log_debug("wt pause")
            save(timer)
            print_message_if_not_silent(timer, "Timer paused.")
            print_check_if_verbose(timer)
        case _:
            print(f"Unhandled status: {timer.status}")


def check():
    timer = load()

    running_minutes = 0

    if timer.status == Status.Running:
        running_minutes = calculate_current_minutes(timer)
    elif timer.status == Status.Paused:
        running_minutes = timer.paused_minutes

    total_minutes = running_minutes + timer.completed_minutes()

    running_str = ""
    match timer.status:
        case Status.Running:
            running_str = hour_minute_str_from_minutes(running_minutes)
        case Status.Paused:
            running_str = hour_minute_str_from_minutes(timer.paused_minutes)
        case Status.Stopped:
            running_str = "--:--"
        case _:
            print(f"Unhandled status: {timer.status}.")
            return

    status_str = timer.status.upper()
    total_str = hour_minute_str_from_minutes(total_minutes)

    print(f"{running_str} {status_str} (total {total_str})")


def history(log_type: str = None):
    valid_types = ["info", "debug"]
    if log_type != None and log_type not in valid_types:
        print(f"Invalid log type: {log_type}. Use one of: {valid_types}")
        quit()

    timer = load()
    
    # Default to info-log if no type specified
    if log_type == "debug":
        path = debug_log_file_path()
    else:
        path = info_log_file_path()
    
    with open(path, "r") as file:
        for line in file:
            print(line, end='')

    # If viewing info-log and timer is running or paused, show current active cycle
    if log_type != "debug" and timer.status in [Status.Running, Status.Paused]:
        current_minutes = calculate_current_minutes(timer)
        total_minutes = current_minutes + timer.completed_minutes()

        if timer.status == Status.Running:
            start_time_only = dt.strptime(timer.start_datetime_str, DT_FORMAT).strftime(TIME_ONLY_FORMAT)
            current_str = mintues_to_hour_minute_str(current_minutes)
            total_str = mintues_to_hour_minute_str(total_minutes)
            print(f"[{start_time_only} =>   ..... ] {'Work timer running:':<22} {current_str} ({total_str})")
        elif timer.status == Status.Paused:
            # Calculate when it originally started based on paused time
            start_dt = dt.now() - timedelta(minutes=timer.paused_minutes)
            start_time_only = start_dt.strftime(TIME_ONLY_FORMAT)
            current_str = mintues_to_hour_minute_str(timer.paused_minutes)
            total_str = mintues_to_hour_minute_str(total_minutes)
            print(f"[{start_time_only} =>   ..... ] {'Work timer paused:':<22} {current_str} ({total_str})")


def add(time: str):
    timer = load()
    validate_timestring_or_quit(time)
    minutes = string_time_to_minutes(time)

    if timer.status == Status.Stopped:
        print("Cannot add time when stopped. Start the timer first.")
        return

    # Backdate the start time by the added minutes
    if timer.status == Status.Running:
        start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
        new_start_dt = start_dt - timedelta(minutes=minutes)
        timer.start_datetime_str = new_start_dt.strftime(DT_FORMAT)
    elif timer.status == Status.Paused:
        timer.paused_minutes += minutes

    log_debug(f"wt add {time}")
    save(timer)


def sub(time: str):
    timer = load()
    validate_timestring_or_quit(time)
    minutes = string_time_to_minutes(time)

    if timer.status == Status.Stopped:
        print("Cannot subtract time when stopped.")
        return

    current_minutes = calculate_current_minutes(timer)
    if current_minutes < minutes:
        print("Cannot reduce current minutes to below 0.")
        return

    # Forward-date the start time by the subtracted minutes (reducing total time)
    if timer.status == Status.Running:
        start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
        new_start_dt = start_dt + timedelta(minutes=minutes)
        timer.start_datetime_str = new_start_dt.strftime(DT_FORMAT)
    elif timer.status == Status.Paused:
        timer.paused_minutes -= minutes

    log_debug(f"wt sub {time}")
    save(timer)


def next_timer():
    stop()
    start()


def reset(msg: str = "Timer reset."):
    old_mode = None
    if os.path.exists(output_file_path()):
        old_timer = load()
        yes_or_no_prompt("Reset timer?")
        old_mode = old_timer.mode

    output_folder = output_folder_path()
    if os.path.exists(output_folder):
        shutil.rmtree(output_folder)

    os.mkdir(output_folder)

    open(debug_log_file_path(), 'a').close()
    open(info_log_file_path(), 'a').close()

    timer = Timer()
    if old_mode:
        timer.mode = old_mode

    save(timer)
    print_message_if_not_silent(timer, msg)
    print_check_if_verbose(timer)


def restart(start_time: str):
    if start_time:
        validate_timestring_or_quit(start_time)
    reset()
    start(start_time)


def new():
    reset("New timer initialized.")


def remove():
    timer = load()
    yes_or_no_prompt("Remove timer?")
    # TODO: Maybe remove whole OUTPUT_FOLDER? Only .wt and not output root because it might break system?
    os.remove(output_file_path())
    os.remove(debug_log_file_path())
    os.remove(info_log_file_path())
    print_message_if_not_silent(timer, "Timer removed.")


def status():
    if not os.path.exists(output_file_path()):
        print(Status.Stopped)
        return
    timer = load()
    print(timer.status)


def mode_select(mode: Mode):
    if mode not in [Mode.Silent, Mode.Normal, Mode.Verbose]:
        print(f"Unhandled mode: {mode}")
        return

    timer = load()
    timer.mode = mode
    save(timer)
    print_message_if_not_silent(timer, f"Timer mode set to {timer.mode}")


def debug():
    path = output_file_path()
    print(f"output_file_path() = {path}\nDT_FORMAT = {DT_FORMAT}")
    if os.path.exists(output_file_path()):
        timer = load()
        print(timer)
    else:
        print(f"No file at {output_file_path()}")


def print_help():
    print("""usage: wt <cmd> [args...]
    Work timer used to time cycles of work. Useful for pomodoro or similar
    work/break cycles. Total time is the sum of currently running/paused
    cycle and previously completed cycles. Cycles can also be thought
    of as laps in a traditional timer.

    Commands:
        start [time]        Starts a new timer or continues paused timer.
                            Optionally add time to set.

        pause               Pauses currently running timer.

        stop                Stops running or paused timer, sets total time,
                            and resets current time.

        check               Prints current and total time along with status.
                            Running wt without any command does the same.

        add <time>          Add <time> to current cycle time (only when running/paused).
                            Backdates start time to reflect actual work time.
                            Time format: 1-4 digit HHMM, HMM, MM, or M.

        sub <time>          Subtract <time> from current cycle time (only when running/paused).
                            Forward-dates start time to reflect actual work time.
                            Time format: 1-4 digit HHMM, HMM, MM, or M.

        log [type]          Show log of timer activity. Defaults to info log.
                            Use 'debug' to see command execution timestamps.
            types:
                info        Activity log with actual work times (default)
                debug       Command execution log with timestamps

        next                Stop current timer and start next.

        reset               Stops and sets current and total timers to zero.

        restart [time]      Reset and start new timer. Optionally add time to set.

        new                 Creates a new timer. Alias for "reset".

        remove              Deletes the timer and related file.

        mode <type>         Change output verbosity.
            types:
                silent      Only prints errors (Default)
                normal      Prints message after performed action.
                verbose     Normal + runs "check" command after other commands.

        help                Prints this help message.

        debug               Prints debug info.
""")


def hour_minute_str_from_minutes(minutes: int) -> str:
    h = minutes//60
    m = minutes % 60

    return f"{h:01d}h {m:02d}m"


def delta_minutes(start: dt, now: dt) -> int:
    delta = now - start

    seconds = delta.total_seconds()
    minutes = int(seconds // 60)

    return minutes


def total_with_paused_str(timer: Timer) -> str:
    total = timer.paused_minutes + timer.completed_minutes()

    return hour_minute_str_from_minutes(total)


def hour_minute_to_minutes(hours: int, minutes: int) -> int:
    return hours * 60 + minutes


def calculate_current_minutes(timer: Timer) -> int:
    return timer.paused_minutes + delta_minutes(dt.strptime(
        timer.start_datetime_str, DT_FORMAT), dt.now())


def save(timer: Timer):
    if not os.path.exists(OUTPUT_FOLDER):
        os.makedirs(OUTPUT_FOLDER)

    data = {
        "status": timer.status,
        "start_datetime_str": timer.start_datetime_str,
        "stop_datetime_str": timer.stop_datetime_str,
        "paused_minutes": timer.paused_minutes,
        "mode": timer.mode,
        "timeline": timer.timeline,
    }

    json_obj = json.dumps(data, indent=4)

    with open(output_file_path(), "w") as file:
        file.write(json_obj)


def load() -> Timer:
    if not os.path.exists(output_file_path()):
        print("No timer exists.")
        quit()

    with open(output_file_path(), "r") as file:
        data = json.load(file)

    return Timer(
        Status(data["status"]),
        data["start_datetime_str"],
        data["stop_datetime_str"],
        data["paused_minutes"],
        data["mode"],
        data.get("timeline", []))


def string_time_to_minutes(time: str) -> int:
    hour = 0
    minute = 0
    match len(time):
        case 4:
            hour = int(time[:2])
            minute = int(time[2:])
        case 3:
            hour = int(time[:1])
            minute = int(time[1:])
        case 2 | 1:
            minute = int(time)

    return hour_minute_to_minutes(hour, minute)


def print_message_if_not_silent(timer: Timer, message: str):
    if timer.mode != Mode.Silent:
        print(message)


def print_check_if_verbose(timer: Timer):
    if timer.mode == Mode.Verbose:
        check()


# Return if user input yes, else quit.
def yes_or_no_prompt(msg: str):
    answer = input(f"{msg} y / n [n]: ")
    if answer.lower() != "y":
        quit()


def validate_timestring_or_quit(time: str):
    if len(time) < 1 or len(time) > 4 or not time.isdigit():
        print("Incorrect time format. Should be 1-4 digit HHMM.")
        quit()

    if len(time) >= 2:
        minutes = int(time[-2:])
        if minutes > 59:
            print(f"Time string minutes are greater than 59. Increase hours instead.")
            quit()


def output_file_path() -> str:
    return f"{project_root_path()}/{OUTPUT_FILE_PATH}"


def debug_log_file_path() -> str:
    return f"{project_root_path()}/{DEBUG_LOG_PATH}"


def info_log_file_path() -> str:
    return f"{project_root_path()}/{INFO_LOG_PATH}"


def project_root_path() -> str:
    if "WT_ROOT" not in os.environ:
        print("Env $WT_ROOT not set.")
        quit()

    return os.environ['WT_ROOT']


def output_folder_path() -> str:
    return f"{project_root_path()}/{OUTPUT_FOLDER}"


def mintues_to_hour_minute_str(mins: int) -> str:
    h = mins//60
    m = mins % 60
    return f"{h}h:{m:02d}m"


def log_debug(msg: str):
    timestamp = dt.now().strftime(DT_FORMAT)
    with open(debug_log_file_path(), "a") as file:
        file.write(f"[{timestamp}] {msg}\n")


def log_info(msg: str):
    with open(info_log_file_path(), "a") as file:
        file.write(f"{msg}\n")


if __name__ == "__main__":
    main()
