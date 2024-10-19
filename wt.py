##
# Work Time (like "time to work" and actually "timing" work :D)
##

from datetime import datetime as dt
from enum import StrEnum
import sys
import os

# TODO: Maybe store tmp file elsewhere to not lose on potential mid-day system reboot? /Users/Shared?? Nice if platform agnostic. Could also auto determine based on platform.
TMP_FILE_PATH = "/tmp/wt"
DT_FORMAT = "%Y-%m-%d %H-%M-%S"


class Status(StrEnum):
    Stopped = "stopped"
    Paused = "paused"
    Running = "running"


class Mode(StrEnum):
    Silent = "silent"
    Normal = "normal"
    Verbose = "verbose"


class Timer():
    def __init__(self, status=Status.Stopped, start="", pausedTime=0, totalTime=0, mode=Mode.Silent):
        self.status: Status = status
        self.start_datetime_str: str = start
        self.paused_minutes: int = pausedTime
        self.completed_minutes: int = totalTime
        self.mode: Mode = mode

    def __str__(self):
        return f"status = {self.status}\nstart_datetime = {self.start_datetime_str}\npaused_minutes = {self.paused_minutes}\ncompleted_minutes = {self.completed_minutes}\nmode = {self.mode}"


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
        case "reset":
            reset()
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
    if os.path.exists(TMP_FILE_PATH):
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

    now = dt.now().strftime(DT_FORMAT)
    timer.start_datetime_str = now
    prev_status = timer.status
    timer.status = Status.Running

    save(timer)
    print_message_if_not_silent(timer, message)
    print_check_if_verbose(timer)

    if start_time != None:
        if prev_status != Status.Stopped:
            print("Can only set start time if stopped")
            return
        else:
            set_timer("current", start_time)


def stop():
    timer = load()
    match timer.status:
        case Status.Stopped:
            print("Timer already stopped.")
        case Status.Running | Status.Paused:
            if timer.status == Status.Running:
                timer.completed_minutes += delta_minutes(
                    dt.strptime(timer.start_datetime_str, DT_FORMAT), dt.now())

            timer.start_datetime_str = ""
            timer.completed_minutes += timer.paused_minutes
            timer.paused_minutes = 0
            timer.status = Status.Stopped
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


def set_timer(type: str, time: str):
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
        timer.paused_minutes += minutes

        if timer.status == Status.Running:
            now = dt.now().strftime(DT_FORMAT)
            timer.start_datetime_str = now

        elif timer.status == Status.Stopped:
            timer.status = Status.Paused

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

        timer.paused_minutes -= minutes

        if timer.status == Status.Running:
            now = dt.now().strftime(DT_FORMAT)
            timer.start_datetime_str = now

    save(timer)


def reset(msg: str = "Timer reset."):
    old_mode = None
    if os.path.exists(TMP_FILE_PATH):
        old_timer = load()
        old_mode = old_timer.mode

    timer = Timer()
    if old_mode:
        timer.mode = old_mode

    save(timer)
    print_message_if_not_silent(timer, msg)
    print_check_if_verbose(timer)


def new():
    reset("New timer initialized.")


def remove():
    timer = load()
    os.remove(TMP_FILE_PATH)
    print_message_if_not_silent(timer, "Timer removed.")


def status():
    if not os.path.exists(TMP_FILE_PATH):
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
    print(f"TMP_FILE_PATH = {TMP_FILE_PATH}\nDT_FORMAT = {DT_FORMAT}")
    if os.path.exists(TMP_FILE_PATH):
        timer = load(debug=True)
        print(timer)
    else:
        print(f"No file at {TMP_FILE_PATH}")


def print_help():
    print("""usage: wt <cmd> [args...]
    Work timer used to time cycles of work. Useful for pomodoro or similar
    work/break cycles. Total time is the sum of currently running/paused
    cycle and previously completed cycles. Cycles can also be thought
    of as laps in a traditional timer.
        
    Commands:
        start               Starts a new timer or continues paused timer.

        pause               Pauses currently running timer.
        
        stop                Stops running or paused timer, sets total time,
                            and resets current time.
        
        check <time>        Prints current and total time along with status.
                            Optionally add time to set.
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

        reset               Stops and sets current and total timers to zero.

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
    # CSV Format = status,startTime,pausedMinutes,totalMinutes,mode
    with open(TMP_FILE_PATH, 'w') as file:
        file.write(f"{timer.status},{timer.start_datetime_str},{
                   timer.paused_minutes},{timer.completed_minutes},{timer.mode}")


def load(debug: bool = False) -> Timer:
    if not os.path.exists(TMP_FILE_PATH):
        print("No timer exists.")
        quit()

    # CSV Format = status,startTime,pausedMinutes,totalMinutes,mode
    line = ""
    with open(TMP_FILE_PATH, 'r') as file:
        line = file.readline().strip('\n')

    if debug:
        print(f'CSV = "{line}"')

    csv = line.split(",")

    return Timer(Status(csv[0]), csv[1], int(csv[2]), int(csv[3]), csv[4])


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


def validate_timestring_or_quit(time: str):
    if len(time) < 1 or len(time) > 4 or not time.isdigit():
        print("Incorrect time format. Should be 1-4 digit HHMM.")
        quit()


if __name__ == "__main__":
    main()
