##
# Work Time (like "time to work" and actually "timing" work :D)
##

from datetime import datetime as dt
from enum import StrEnum
import sys
import os
import shutil
import re
import json

# Keep updated with .gitignore !
OUTPUT_FOLDER = ".out"
OUTPUT_FILE_NAME = "wt.json"
OUTPUT_LOG_NAME = "log"
OUTPUT_LOG_PATH = f"{OUTPUT_FOLDER}/{OUTPUT_LOG_NAME}"
OUTPUT_FILE_PATH = f"{OUTPUT_FOLDER}/{OUTPUT_FILE_NAME}"

DT_FORMAT = "%Y-%m-%d %H:%M:%S"


class Status(StrEnum):
    Stopped = "stopped"
    Paused = "paused"
    Running = "running"


class Mode(StrEnum):
    Silent = "silent"
    Normal = "normal"
    Verbose = "verbose"


class LogType(StrEnum):
    INFO = "INF"
    COMMAND = "CMD"


class Timer():
    def __init__(self, status=Status.Stopped, start="", stop="", pausedTime=0, totalTime=0, mode=Mode.Silent):
        self.status: Status = status
        self.start_datetime_str: str = start
        self.stop_datetime_str: str = stop
        self.paused_minutes: int = pausedTime
        self.completed_minutes: int = totalTime
        self.mode: Mode = mode

    def __str__(self):
        return f"status = {self.status}\nstart_datetime = {self.start_datetime_str}\nstop_datetime = {self.stop_datetime_str}\npaused_minutes = {self.paused_minutes}\ncompleted_minutes = {self.completed_minutes}\nmode = {self.mode}"


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
        case "set":
            if len(args) != 3:
                print("Incorrect amount of arguments.")
                return
            set_timer(args[1], args[2])
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
    timer = Timer()

    if start_time:
        validate_timestring_or_quit(start_time)

    if os.path.exists(output_file_path()):
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
        break_mins = delta_minutes(
            dt.strptime(timer.stop_datetime_str, DT_FORMAT), dt.now())
        break_time_str = mintues_to_hour_minute_str(break_mins)

    timer.stop_datetime_str = ""
    now = dt.now().strftime(DT_FORMAT)
    timer.start_datetime_str = now
    prev_status = timer.status
    timer.status = Status.Running

    start_time_log = f" {start_time}" if start_time != None else ""
    log(LogType.COMMAND, f"wt start{start_time_log}")
    if break_time_str != "":
        log(LogType.INFO, f"Started again after: {break_time_str}")

    save(timer)
    print_message_if_not_silent(timer, message)
    print_check_if_verbose(timer)

    if start_time != None:
        if prev_status != Status.Stopped:
            print("Can only set start time if stopped")
            return
        else:
            set_timer("current", start_time, False)


def stop():
    timer = load()
    cycle_minutes = 0
    match timer.status:
        case Status.Stopped:
            print("Timer already stopped.")
        case Status.Running | Status.Paused:
            if timer.status == Status.Running:
                cycle_minutes += delta_minutes(
                    dt.strptime(timer.start_datetime_str, DT_FORMAT), dt.now())

            timer.stop_datetime_str = dt.now().strftime(DT_FORMAT)
            timer.start_datetime_str = ""
            cycle_minutes += timer.paused_minutes
            timer.completed_minutes += cycle_minutes
            timer.paused_minutes = 0
            timer.status = Status.Stopped

            log(LogType.COMMAND, "wt stop")
            cycle_str = mintues_to_hour_minute_str(cycle_minutes)
            total_str = mintues_to_hour_minute_str(timer.completed_minutes)
            log(LogType.INFO, f"Completed cycle: {cycle_str} ({total_str})")
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

            log(LogType.COMMAND, "wt pause")
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

    total_minutes = running_minutes + timer.completed_minutes

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


def history(log_type: LogType = None):
    filters = ["info", "cmd"]
    if log_type != None and log_type not in filters:
        print(f"Invalid log type filter: {log_type}. Use one of: {filters}")
        quit()

    load()  # Make sure there is a timer.
    path = log_file_path()
    with open(path, "r") as file:
        for line in file:
            if log_type:
                lt = log_type_from_log_line(line)
                if (lt == LogType.INFO and log_type != "info") or (lt == LogType.COMMAND and log_type != "cmd"):
                    continue

            print(line, end='')


def set_timer(type: str, time: str, should_log: bool = True):
    timer = load()
    validate_timer_type_or_quit(type)
    validate_timestring_or_quit(time)
    minutes = string_time_to_minutes(time)

    match type:
        case "total" | "t":
            if timer.status != Status.Stopped:
                print("Can only set total time when timer is stopped.")
                return

            timer.completed_minutes = minutes
        case "current" | "c":
            if timer.status not in [Status.Running, Status.Paused, Status.Stopped]:
                print(f"Current status {timer.status} not handled.")
                return

            timer.paused_minutes = minutes

            if timer.status == Status.Running:
                now = dt.now().strftime(DT_FORMAT)
                timer.start_datetime_str = now

            elif timer.status == Status.Stopped:
                timer.status = Status.Paused

        case _:
            print(f"Unhandled type: {type}.")
            return

    if should_log:
        log(LogType.COMMAND, f"wt set {type} {time}")

    save(timer)
    print_message_if_not_silent(timer, "Timer set.")
    print_check_if_verbose(timer)


def add(time: str):
    timer = load()
    validate_timestring_or_quit(time)
    minutes = string_time_to_minutes(time)

    type = "total" if timer.status == Status.Stopped else "current"

    if type == "total":
        if timer.status != Status.Stopped:
            print("Can only update total time when timer is stopped.")
            return

        timer.completed_minutes += minutes

    else:
        timer.paused_minutes = calculate_current_minutes(timer) + minutes

        if timer.status == Status.Running:
            now = dt.now().strftime(DT_FORMAT)
            timer.start_datetime_str = now

        elif timer.status == Status.Stopped:
            timer.status = Status.Paused

    log(LogType.COMMAND, f"wt add {time}")
    save(timer)


def sub(time: str):
    timer = load()
    validate_timestring_or_quit(time)
    minutes = string_time_to_minutes(time)

    type = "total" if timer.status == Status.Stopped else "current"

    if type == "total":
        if timer.completed_minutes < minutes:
            print("Cannot reduce total minutes to below 0.")
            return

        timer.completed_minutes -= minutes

    else:
        new_current_minutes = calculate_current_minutes(timer) - minutes
        if new_current_minutes < 0:
            print("Cannot reduce current minutes to below 0.")
            return

        timer.paused_minutes = new_current_minutes

        if timer.status == Status.Running:
            now = dt.now().strftime(DT_FORMAT)
            timer.start_datetime_str = now

    log(LogType.COMMAND, f"wt sub {time}")
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

    open(log_file_path(), 'a').close()

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
    os.remove(log_file_path())
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

        set <type> <time>   Manually set total/current time using 1-4 digit
                            HHMM, HMM, MM, or M. Ex. wt set total 15 = 15min.
                            types:
                                total (t)
                                current (c)

        add <time>          Add <time> to total if stopped, else current time.
                            Same time format as Set command.

        sub <time>          Subtract <time> to total if stopped, else current time.
                            Same time format as Set command.

        log [type]          Show log of successfully run commands which impacted
                            the timer. Optionally filter logs by type.
            types:
                info        Only prints info logs
                cmd         Only prints logs of commands

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
    total = timer.paused_minutes + timer.completed_minutes

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
        "completed_minutes": timer.completed_minutes,
        "mode": timer.mode,
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
        data["start_datetime_str"],
        data["paused_minutes"],
        data["completed_minutes"],
        data["mode"])


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


def validate_timer_type_or_quit(type: str):
    if type not in ["total", "current", "t", "c"]:
        print("Incorrect timer type. Should be 'total' or 'current'.")
        quit()


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


def log_file_path() -> str:
    return f"{project_root_path()}/{OUTPUT_LOG_PATH}"


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


def log(log_type: LogType,  msg: str):
    timestamp = dt.now().strftime(DT_FORMAT)
    with open(log_file_path(), "a") as file:
        file.write(f"[{timestamp}] [{log_type}] {msg}\n")


def log_type_from_log_line(line: str) -> LogType:
    pattern = re.compile("\\] \\[(.*?)\\]")
    res = pattern.findall(line)
    if len(res) != 1:
        print(f"Issue extracting log type from: {line}")
        quit()

    try:
        t = LogType(res[0])
        return t
    except:
        print(f"Issue extracting log type from: {line}")
        print(f"Invalid log type: {res[0]}")
        quit()


if __name__ == "__main__":
    main()
