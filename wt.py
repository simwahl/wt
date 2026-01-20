##
# Work Time (like "time to work" and actually "timing" work :D)
##

from datetime import datetime as dt, timedelta
from enum import StrEnum
from typing import List
import sys
import os
import shutil
import json

# Keep updated with .gitignore !
OUTPUT_FOLDER = ".out"
OUTPUT_FILE_NAME = "wt.json"
DEBUG_LOG_NAME = "debug-log"
INFO_LOG_NAME = "info-log"
DAILY_REPORT_NAME = "daily-reports"
DEBUG_LOG_PATH = f"{OUTPUT_FOLDER}/{DEBUG_LOG_NAME}"
INFO_LOG_PATH = f"{OUTPUT_FOLDER}/{INFO_LOG_NAME}"
OUTPUT_FILE_PATH = f"{OUTPUT_FOLDER}/{OUTPUT_FILE_NAME}"
DAILY_REPORT_PATH = f"{OUTPUT_FOLDER}/{DAILY_REPORT_NAME}"

DT_FORMAT = "%Y-%m-%d %H:%M"
TIME_ONLY_FORMAT = "%H:%M"


def get_current_time():
    """Get current time, or mock time if WT_MOCK_TIME is set."""
    mock_time = os.environ.get("WT_MOCK_TIME")
    if mock_time:
        return dt.strptime(mock_time, DT_FORMAT)
    return dt.now()


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
            start="", stop="", accumulatedMinutes=0,
            mode=Mode.Silent,
            timeline=None,
            dayStart="",
            cycleStart=""):
        self.status: Status = status
        self.start_datetime_str: str = start  # When current running segment started (or pause started)
        self.stop_datetime_str: str = stop
        # Paused time within current cycle
        self.paused_minutes: int = accumulatedMinutes
        self.mode: Mode = mode
        # Timeline entries: {"type": "work"|"break", "minutes": N}
        self.timeline: List[dict] = timeline if timeline is not None else []
        # When did this day's work start (first work cycle start time)
        self.day_start: str = dayStart
        # When did the current cycle start (first start, before any pauses)
        self.cycle_start_datetime_str: str = cycleStart

    def __str__(self):
        return (
            f"status = {self.status}\n"
            f"day_start = {self.day_start}\n"
            f"cycle_start_datetime_str = {self.cycle_start_datetime_str}\n"
            f"start_datetime_str = {self.start_datetime_str}\n"
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
                total += entry["minutes"]
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
        case "log":
            log_type = None if len(args) < 2 else args[1]
            history(log_type)
        case "mod":
            if len(args) == 1:
                mod_list()
            elif len(args) == 3 and args[2] == "drop":
                # wt mod N drop
                mod_drop(args[1])
            elif len(args) == 4 and args[1] == "start":
                # wt mod start add/sub time
                mod_start(args[2], args[3])
            elif len(args) == 4:
                # wt mod N add/sub time
                mod_duration(args[1], args[2], args[3])
            else:
                print("Incorrect arguments. Usage:")
                print("  wt mod                      - list cycles")
                print("  wt mod start <add|sub> <time> - adjust day start time")
                print("  wt mod <num> <add|sub> <time> - adjust cycle duration")
                print("  wt mod <num> drop            - remove cycle and merge")
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
        case "report":
            report()
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
            # Calculate pause duration and add to paused_minutes
            pause_start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
            pause_duration = delta_minutes(pause_start_dt, get_current_time())
            timer.paused_minutes += pause_duration
        case Status.Stopped:
            message = "Starting timer."

    # Track if this is first cycle (before adding break)
    is_first_cycle = len(timer.timeline) == 0

    # If start_time is provided on subsequent cycle, validate break duration first
    if start_time != None and not is_first_cycle:
        minutes = string_time_to_minutes(start_time)
        # Calculate what the break would be
        if timer.stop_datetime_str != "":
            break_start_dt = dt.strptime(timer.stop_datetime_str, DT_FORMAT)
            break_stop_dt = get_current_time()
            break_mins = delta_minutes(break_start_dt, break_stop_dt)
            
            if break_mins < minutes:
                print(f"Cannot reduce break below 0. Break was {mintues_to_hour_minute_str(break_mins)}, tried to subtract {mintues_to_hour_minute_str(minutes)}.")
                return

    # Calculate break if resuming from stopped state
    if timer.stop_datetime_str != "":
        break_start_dt = dt.strptime(timer.stop_datetime_str, DT_FORMAT)
        break_stop_dt = get_current_time()
        break_mins = delta_minutes(break_start_dt, break_stop_dt)
        timer.timeline.append({
            "type": "break",
            "minutes": break_mins
        })

    timer.stop_datetime_str = ""
    now = get_current_time()
    timer.start_datetime_str = now.strftime(DT_FORMAT)
    
    # If this is the first cycle of the day, set day_start
    if not timer.day_start:
        timer.day_start = timer.start_datetime_str
    
    # If starting a new cycle (not resuming from pause), set cycle_start
    if timer.status == Status.Stopped:
        timer.cycle_start_datetime_str = timer.start_datetime_str
    
    timer.status = Status.Running

    start_time_log = f" {start_time}" if start_time != None else ""
    log_debug(f"wt start{start_time_log}")

    save(timer)
    print_message_if_not_silent(timer, message)
    print_check_if_verbose(timer)

    # Handle start_time parameter
    if start_time != None:
        minutes = string_time_to_minutes(start_time)
        timer = load()
        
        if is_first_cycle:
            # First cycle: backdate day_start, start_datetime_str and cycle_start
            start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
            new_start_dt = start_dt - timedelta(minutes=minutes)
            
            # Validate: new start must be before now
            if new_start_dt >= now:
                print(f"Cannot backdate start that far.")
                return
            
            timer.start_datetime_str = new_start_dt.strftime(DT_FORMAT)
            timer.cycle_start_datetime_str = new_start_dt.strftime(DT_FORMAT)
            timer.day_start = new_start_dt.strftime(DT_FORMAT)
            
            save(timer)
        else:
            # Subsequent cycles: reduce the break we just added
            if timer.timeline and timer.timeline[-1]["type"] == "break":
                break_entry = timer.timeline[-1]
                new_break_mins = break_entry["minutes"] - minutes
                
                # This should never happen due to validation above, but keep as safety
                if new_break_mins < 0:
                    print(f"Cannot reduce break below 0. Break was {mintues_to_hour_minute_str(break_entry['minutes'])}, tried to subtract {mintues_to_hour_minute_str(minutes)}.")
                    return
                
                break_entry["minutes"] = new_break_mins
                
                # Backdate start_datetime_str and cycle_start to reflect earlier start
                start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
                new_start_dt = start_dt - timedelta(minutes=minutes)
                timer.start_datetime_str = new_start_dt.strftime(DT_FORMAT)
                timer.cycle_start_datetime_str = new_start_dt.strftime(DT_FORMAT)
                
                # Regenerate info-log to reflect the change
                regenerate_info_log(timer)
                save(timer)
    else:
        # No time parameter, but we may have added a break - regenerate if needed
        timer = load()
        if timer.timeline and timer.timeline[-1]["type"] == "break":
            regenerate_info_log(timer)
            save(timer)


def stop():
    timer = load()
    match timer.status:
        case Status.Stopped:
            print("Timer already stopped.")
        case Status.Running | Status.Paused:
            now = get_current_time()
            stop_time_str = now.strftime(DT_FORMAT)
            
            # Calculate work duration: total_cycle_time - paused_time
            if timer.status == Status.Paused:
                # Add current pause duration
                pause_start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
                current_pause = delta_minutes(pause_start_dt, now)
                total_paused = timer.paused_minutes + current_pause
            else:
                total_paused = timer.paused_minutes
            
            # Total cycle time from first start to now
            cycle_start_dt = dt.strptime(timer.cycle_start_datetime_str, DT_FORMAT)
            total_cycle_time = delta_minutes(cycle_start_dt, now)
            
            # Work time = total cycle time - paused time
            cycle_minutes = total_cycle_time - total_paused
            
            # Ensure we don't go below 0
            if cycle_minutes < 0:
                cycle_minutes = 0
            
            # Add work entry to timeline with work, paused, and total time
            timer.timeline.append({
                "type": "work",
                "minutes": cycle_minutes,  # Actual work time
                "paused_minutes": total_paused,  # Time paused
                "total_minutes": total_cycle_time  # Total cycle time (work + paused)
            })
            
            timer.stop_datetime_str = stop_time_str
            timer.start_datetime_str = ""
            timer.cycle_start_datetime_str = ""
            timer.paused_minutes = 0
            timer.status = Status.Stopped

            log_debug("wt stop")
            
            # Calculate durations for logging
            cycle_str = mintues_to_hour_minute_str(cycle_minutes)
            total_str = mintues_to_hour_minute_str(timer.completed_minutes())
            
            # Calculate start and end times from timeline
            if timer.day_start:
                start_dt = dt.strptime(timer.day_start, DT_FORMAT)
                # Sum all previous entries to get current cycle start
                for i in range(len(timer.timeline) - 1):  # -1 because we just added the work entry
                    prev_entry = timer.timeline[i]
                    if prev_entry["type"] == "work":
                        # Use total_minutes for work entries (includes pauses)
                        entry_duration = prev_entry.get("total_minutes", prev_entry["minutes"])
                    else:
                        entry_duration = prev_entry["minutes"]
                    start_dt += timedelta(minutes=entry_duration)
                end_dt = start_dt + timedelta(minutes=total_cycle_time)
                
                start_time_only = start_dt.strftime(TIME_ONLY_FORMAT)
                end_time_only = end_dt.strftime(TIME_ONLY_FORMAT)
                
                # Check if dates differ
                day_diff = (end_dt.date() - start_dt.date()).days
                day_indicator = f"  [+{day_diff} day]" if day_diff > 0 else ""
                
                # Include pause time if present
                if total_paused > 0:
                    paused_str = mintues_to_hour_minute_str(total_paused)
                    log_info(f"[{start_time_only} => {end_time_only}] Work: {cycle_str}, Paused: {paused_str} ({total_str}){day_indicator}")
                else:
                    log_info(f"[{start_time_only} => {end_time_only}] Work: {cycle_str} ({total_str}){day_indicator}")
            else:
                if total_paused > 0:
                    paused_str = mintues_to_hour_minute_str(total_paused)
                    log_info(f"Work: {cycle_str}, Paused: {paused_str} ({total_str})")
                else:
                    log_info(f"Work: {cycle_str} ({total_str})")
            
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
            # Record when the pause started (in start_datetime_str for resume to calculate duration)
            timer.start_datetime_str = get_current_time().strftime(DT_FORMAT)
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

    if timer.status == Status.Running or timer.status == Status.Paused:
        running_minutes = calculate_current_minutes(timer)

    total_minutes = running_minutes + timer.completed_minutes()

    running_str = ""
    match timer.status:
        case Status.Running:
            running_str = hour_minute_str_from_minutes(running_minutes)
        case Status.Paused:
            running_str = hour_minute_str_from_minutes(running_minutes)
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
    
    # Print with line numbers for info-log, plain for debug-log
    if log_type == "debug":
        with open(path, "r") as file:
            for line in file:
                print(line, end='')
    else:
        with open(path, "r") as file:
            for i, line in enumerate(file, 1):
                print(f"{i:02d}. {line}", end='')

    # If viewing info-log and timer is running or paused, show current active cycle
    if log_type != "debug" and timer.status in [Status.Running, Status.Paused]:
        current_minutes = calculate_current_minutes(timer)
        total_minutes = current_minutes + timer.completed_minutes()
        
        current_str = mintues_to_hour_minute_str(current_minutes)
        total_str = mintues_to_hour_minute_str(total_minutes)
        
        # Calculate when this cycle started
        if timer.day_start:
            cycle_start_dt = dt.strptime(timer.day_start, DT_FORMAT)
            for entry in timer.timeline:
                cycle_start_dt += timedelta(minutes=entry["minutes"])
        else:
            cycle_start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT) if timer.start_datetime_str else get_current_time()
        
        start_time_only = cycle_start_dt.strftime(TIME_ONLY_FORMAT)
        
        # Check if crossed midnight
        now = get_current_time()
        day_diff = (now.date() - cycle_start_dt.date()).days
        day_indicator = f"  [+{day_diff} day]" if day_diff > 0 else ""
        
        # Line number is timeline length + 1 (for current active cycle)
        line_num = len(timer.timeline) + 1
        
        if timer.status == Status.Running:
            print(f"{line_num:02d}. [{start_time_only} => .....] Work: {current_str} ({total_str}){day_indicator}")
        elif timer.status == Status.Paused:
            print(f"{line_num:02d}. [{start_time_only} => .....] Work (paused): {current_str} ({total_str}){day_indicator}")


def report():
    """Print a one-line summary of the day's work."""
    timer = load()
    
    if not timer.day_start:
        print("No work recorded today.")
        return
    
    # Calculate totals from timeline
    total_work_mins = 0
    total_break_mins = 0
    total_paused_mins = 0
    
    for entry in timer.timeline:
        if entry["type"] == "work":
            total_work_mins += entry["minutes"]
            total_paused_mins += entry.get("paused_minutes", 0)
        else:
            total_break_mins += entry["minutes"]
    
    # Add current running/paused time if applicable
    current_mins = 0
    if timer.status in [Status.Running, Status.Paused]:
        current_mins = calculate_current_minutes(timer)
        total_work_mins += current_mins
    
    # Calculate end time
    start_dt = dt.strptime(timer.day_start, DT_FORMAT)
    end_dt = start_dt
    
    # Add all timeline entries (using total_minutes for work to include paused time)
    for entry in timer.timeline:
        if entry["type"] == "work":
            end_dt += timedelta(minutes=entry.get("total_minutes", entry["minutes"]))
        else:
            end_dt += timedelta(minutes=entry["minutes"])
    
    # Add current running time
    if timer.status in [Status.Running, Status.Paused]:
        end_dt += timedelta(minutes=current_mins)
    
    # Format output
    date_str = start_dt.strftime("%Y-%m-%d")
    start_time = start_dt.strftime(TIME_ONLY_FORMAT)
    end_time = end_dt.strftime(TIME_ONLY_FORMAT)
    work_str = mintues_to_hour_minute_str(total_work_mins)
    break_str = mintues_to_hour_minute_str(total_break_mins)
    paused_str = mintues_to_hour_minute_str(total_paused_mins)
    total_str = mintues_to_hour_minute_str(total_work_mins + total_break_mins + total_paused_mins)
    
    # Check if crossed midnight
    day_diff = (end_dt.date() - start_dt.date()).days
    day_indicator = f" [+{day_diff} day]" if day_diff > 0 else ""
    
    print(f"{date_str} | {start_time} -> {end_time} | Work: {work_str} | Break: {break_str} | Paused: {paused_str} | Total: {total_str}{day_indicator}")


def mod_list():
    """Show usage help for mod command."""
    print("Usage:")
    print("  wt mod start <add|sub> <time> - adjust day start time")
    print("  wt mod <num> <add|sub> <time> - adjust cycle duration")
    print("  wt mod <num> drop             - remove cycle")


def mod_start(operation: str, time_str: str):
    """Modify day_start timestamp."""
    timer = load()
    
    if timer.status != Status.Stopped:
        print("Cannot modify while timer is running or paused. Stop the timer first.")
        return
    
    if not timer.day_start:
        print("No day_start to modify.")
        return
    
    if operation not in ["add", "sub"]:
        print(f"Invalid operation: {operation}. Use 'add' or 'sub'")
        return
    
    # For mod operations, just parse as simple time string
    if not time_str.isdigit():
        print("Invalid time format. Should be digits only.")
        return
    
    minutes = string_time_to_minutes(time_str)
    
    # sub means earlier start (subtract time from timestamp)
    # add means later start (add time to timestamp)
    day_start_dt = dt.strptime(timer.day_start, DT_FORMAT)
    if operation == "sub":
        new_day_start = day_start_dt - timedelta(minutes=minutes)
    else:
        new_day_start = day_start_dt + timedelta(minutes=minutes)
    
    timer.day_start = new_day_start.strftime(DT_FORMAT)
    
    # If currently running the first work cycle, also adjust start_datetime_str
    if timer.status == Status.Running and timer.start_datetime_str:
        # Check if this is the first work cycle (no work entries in timeline)
        has_work_cycles = any(entry["type"] == "work" for entry in timer.timeline)
        if not has_work_cycles:
            # This is the first cycle - adjust start_datetime_str too
            start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
            if operation == "sub":
                new_start = start_dt - timedelta(minutes=minutes)
            else:
                new_start = start_dt + timedelta(minutes=minutes)
            timer.start_datetime_str = new_start.strftime(DT_FORMAT)
    
    # Regenerate info-log
    regenerate_info_log(timer)
    
    log_debug(f"wt mod start {operation} {time_str}")
    save(timer)
    
    print(f"Day start adjusted by {'+' if operation == 'add' else '-'}{mintues_to_hour_minute_str(minutes)}")


def mod_duration(cycle_num_str: str, operation: str, time_str: str):
    """Modify a specific cycle's duration."""
    timer = load()
    
    if timer.status != Status.Stopped:
        print("Cannot modify while timer is running or paused. Stop the timer first.")
        return
    
    # Validate inputs
    if not cycle_num_str.isdigit():
        print(f"Invalid cycle number: {cycle_num_str}")
        return
    
    cycle_num = int(cycle_num_str)
    if cycle_num < 1 or cycle_num > len(timer.timeline):
        print(f"Cycle {cycle_num} does not exist. Valid range: 1-{len(timer.timeline)}")
        return
    
    if operation not in ["add", "sub"]:
        print(f"Invalid operation: {operation}. Use 'add' or 'sub'")
        return
    
    # For mod operations, just parse as simple time string
    if not time_str.isdigit():
        print("Invalid time format. Should be digits only.")
        return
    
    minutes = string_time_to_minutes(time_str)
    
    # Get the entry to modify (0-indexed)
    entry_idx = cycle_num - 1
    entry = timer.timeline[entry_idx]
    
    # Modify the duration
    if operation == "add":
        entry["minutes"] += minutes
        # For work entries, also update total_minutes (paused stays the same)
        if entry["type"] == "work":
            paused = entry.get("paused_minutes", 0)
            entry["total_minutes"] = entry["minutes"] + paused
    else:  # sub
        new_duration = entry["minutes"] - minutes
        if new_duration < 0:
            print(f"Error: Duration would be negative. Current: {mintues_to_hour_minute_str(entry['minutes'])}")
            return
        entry["minutes"] = new_duration
        # For work entries, also update total_minutes (paused stays the same)
        if entry["type"] == "work":
            paused = entry.get("paused_minutes", 0)
            entry["total_minutes"] = entry["minutes"] + paused
    
    # Regenerate info-log
    regenerate_info_log(timer)
    
    log_debug(f"wt mod {cycle_num_str} {operation} {time_str}")
    save(timer)
    
    time_change = f"{'+' if operation == 'add' else '-'}{mintues_to_hour_minute_str(minutes)}"
    print(f"Modified cycle {cycle_num} duration by {time_change}")


def mod_drop(cycle_num_str: str):
    """Remove a cycle and merge adjacent work cycles if applicable."""
    timer = load()
    
    if timer.status != Status.Stopped:
        print("Cannot modify while timer is running or paused. Stop the timer first.")
        return
    
    # Validate input
    if not cycle_num_str.isdigit():
        print(f"Invalid cycle number: {cycle_num_str}")
        return
    
    cycle_num = int(cycle_num_str)
    if cycle_num < 1 or cycle_num > len(timer.timeline):
        print(f"Cycle {cycle_num} does not exist. Valid range: 1-{len(timer.timeline)}")
        return
    
    entry_idx = cycle_num - 1
    entry = timer.timeline[entry_idx]
    entry_type = entry["type"]
    
    # Check for merge conditions
    # If dropping a break between two work cycles, they merge
    # If dropping a work cycle between two breaks, they merge
    merge_msg = ""
    
    if entry_type == "break":
        # Check if surrounded by work cycles
        has_prev_work = entry_idx > 0 and timer.timeline[entry_idx - 1]["type"] == "work"
        has_next_work = entry_idx < len(timer.timeline) - 1 and timer.timeline[entry_idx + 1]["type"] == "work"
        
        if has_prev_work and has_next_work:
            # Merge: combine the two work cycles
            prev_work_mins = timer.timeline[entry_idx - 1]["minutes"]
            next_work_mins = timer.timeline[entry_idx + 1]["minutes"]
            merged_mins = prev_work_mins + next_work_mins
            
            # Remove the break and next work, extend prev work
            timer.timeline[entry_idx - 1]["minutes"] = merged_mins
            timer.timeline.pop(entry_idx + 1)  # Remove next work
            timer.timeline.pop(entry_idx)      # Remove break
            merge_msg = f" (merged adjacent work cycles: {mintues_to_hour_minute_str(merged_mins)})"
        else:
            # Just remove the break
            timer.timeline.pop(entry_idx)
    else:  # work cycle
        # Check if surrounded by breaks
        has_prev_break = entry_idx > 0 and timer.timeline[entry_idx - 1]["type"] == "break"
        has_next_break = entry_idx < len(timer.timeline) - 1 and timer.timeline[entry_idx + 1]["type"] == "break"
        
        if has_prev_break and has_next_break:
            # Merge: combine the two breaks
            prev_break_mins = timer.timeline[entry_idx - 1]["minutes"]
            next_break_mins = timer.timeline[entry_idx + 1]["minutes"]
            merged_mins = prev_break_mins + next_break_mins
            
            # Remove the work and next break, extend prev break
            timer.timeline[entry_idx - 1]["minutes"] = merged_mins
            timer.timeline.pop(entry_idx + 1)  # Remove next break
            timer.timeline.pop(entry_idx)      # Remove work
            merge_msg = f" (merged adjacent breaks: {mintues_to_hour_minute_str(merged_mins)})"
        else:
            # Just remove the work cycle
            timer.timeline.pop(entry_idx)
    
    # Regenerate info-log
    regenerate_info_log(timer)
    
    log_debug(f"wt mod {cycle_num_str} drop")
    save(timer)
    
    print(f"Removed cycle {cycle_num}{merge_msg}")


def regenerate_info_log(timer: Timer):
    """Regenerate the info-log from the timeline."""
    # Clear the info-log
    with open(info_log_file_path(), "w") as file:
        file.write("")
    
    if not timer.timeline:
        return
    
    # Calculate times from day_start
    if timer.day_start:
        current_time = dt.strptime(timer.day_start, DT_FORMAT)
    else:
        # Fallback if no day_start
        current_time = get_current_time()
    
    running_total = 0
    
    for entry in timer.timeline:
        if entry["type"] == "work":
            start_dt = current_time
            work_mins = entry["minutes"]  # Actual work time
            paused_mins = entry.get("paused_minutes", 0)  # Paused time
            total_mins = entry.get("total_minutes", work_mins + paused_mins)  # Total (backward compat)
            current_time += timedelta(minutes=total_mins)  # Use total time for timestamps
            stop_dt = current_time
            
            running_total += work_mins  # Accumulate only work time
            
            start_time_only = start_dt.strftime(TIME_ONLY_FORMAT)
            end_time_only = stop_dt.strftime(TIME_ONLY_FORMAT)
            cycle_str = mintues_to_hour_minute_str(work_mins)  # Show work time
            total_str = mintues_to_hour_minute_str(running_total)
            
            # Check if dates differ
            day_diff = (stop_dt.date() - start_dt.date()).days
            day_indicator = f"  [+{day_diff} day]" if day_diff > 0 else ""
            
            # Include pause time if present
            if paused_mins > 0:
                paused_str = mintues_to_hour_minute_str(paused_mins)
                log_info(f"[{start_time_only} => {end_time_only}] Work: {cycle_str}, Paused: {paused_str} ({total_str}){day_indicator}")
            else:
                log_info(f"[{start_time_only} => {end_time_only}] Work: {cycle_str} ({total_str}){day_indicator}")
        else:
            # Break entry
            start_dt = current_time
            duration_mins = entry["minutes"]
            current_time += timedelta(minutes=duration_mins)
            stop_dt = current_time
            
            start_time_only = start_dt.strftime(TIME_ONLY_FORMAT)
            end_time_only = stop_dt.strftime(TIME_ONLY_FORMAT)
            cycle_str = mintues_to_hour_minute_str(duration_mins)
            
            log_info(f"[{start_time_only} => {end_time_only}] Break: {cycle_str}")


def next_timer():
    stop()

    # Add a 0-minute break between cycles
    timer = load()
    timer.timeline.append({
        "type": "break",
        "minutes": 0
    })
    
    # Regenerate info-log to include the 0-minute break
    regenerate_info_log(timer)
    save(timer)

    # Start next cycle (skip break calculation since we just added one)
    timer.stop_datetime_str = ""
    now = get_current_time()
    timer.start_datetime_str = now.strftime(DT_FORMAT)
    timer.cycle_start_datetime_str = now.strftime(DT_FORMAT)
    timer.paused_minutes = 0
    timer.status = Status.Running

    log_debug("wt next")
    save(timer)
    print_message_if_not_silent(timer, "Next cycle started.")
    print_check_if_verbose(timer)


def save_daily_report():
    """Save a daily report to the daily-reports file."""
    timer = load()
    
    # Only save if there's work recorded
    if not timer.day_start:
        return
    
    # Calculate totals from timeline
    total_work_mins = 0
    total_break_mins = 0
    
    for entry in timer.timeline:
        if entry["type"] == "work":
            total_work_mins += entry["minutes"]
        else:
            total_break_mins += entry["minutes"]
    
    # Add current running/paused time if applicable
    current_mins = 0
    if timer.status in [Status.Running, Status.Paused]:
        current_mins = calculate_current_minutes(timer)
        total_work_mins += current_mins
    
    # Calculate end time
    start_dt = dt.strptime(timer.day_start, DT_FORMAT)
    end_dt = start_dt
    
    # Add all timeline entries (using total_minutes for work to include paused time)
    for entry in timer.timeline:
        if entry["type"] == "work":
            end_dt += timedelta(minutes=entry.get("total_minutes", entry["minutes"]))
        else:
            end_dt += timedelta(minutes=entry["minutes"])
    
    # Add current running time
    if timer.status in [Status.Running, Status.Paused]:
        end_dt += timedelta(minutes=current_mins)
    
    # Format output
    date_str = start_dt.strftime("%Y-%m-%d")
    start_time = start_dt.strftime(TIME_ONLY_FORMAT)
    end_time = end_dt.strftime(TIME_ONLY_FORMAT)
    work_str = mintues_to_hour_minute_str(total_work_mins)
    break_str = mintues_to_hour_minute_str(total_break_mins)
    total_str = mintues_to_hour_minute_str(total_work_mins + total_break_mins)
    
    # Check if crossed midnight
    day_diff = (end_dt.date() - start_dt.date()).days
    day_indicator = f" [+{day_diff} day]" if day_diff > 0 else ""
    
    report_line = f"{date_str} | {start_time} -> {end_time} | Work: {work_str} | Break: {break_str} | Total: {total_str}{day_indicator}\n"
    
    # Prepend to daily report file (newest at top)
    report_path = daily_report_file_path()
    existing_content = ""
    if os.path.exists(report_path):
        with open(report_path, "r") as file:
            existing_content = file.read()
    
    with open(report_path, "w") as file:
        file.write(report_line + existing_content)


def reset(msg: str = "Timer reset."):
    old_mode = None
    daily_report_content = None
    
    if os.path.exists(output_file_path()):
        old_timer = load()
        yes_or_no_prompt("Reset timer?")
        old_mode = old_timer.mode
        # Save daily report before resetting
        save_daily_report()
        
        # Preserve daily report content before deleting folder
        daily_report_path = daily_report_file_path()
        if os.path.exists(daily_report_path):
            with open(daily_report_path, "r") as f:
                daily_report_content = f.read()

    output_folder = output_folder_path()
    if os.path.exists(output_folder):
        shutil.rmtree(output_folder)

    os.mkdir(output_folder)

    open(debug_log_file_path(), 'a').close()
    open(info_log_file_path(), 'a').close()
    
    # Restore daily report content if it existed
    if daily_report_content:
        with open(daily_report_file_path(), "w") as f:
            f.write(daily_report_content)

    timer = Timer()
    if old_mode:
        timer.mode = old_mode

    save(timer)
    print_message_if_not_silent(timer, msg)
    print_check_if_verbose(timer)


def restart(start_time: str):
    if start_time:
        validate_timestring_or_quit(start_time)
    # Note: reset() will handle saving the daily report
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
    # Remove daily report file if it exists
    if os.path.exists(daily_report_file_path()):
        os.remove(daily_report_file_path())
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

        log [type]          Show log of timer activity. Defaults to info log.
                            Use 'debug' to see command execution timestamps.
            types:
                info        Activity log with actual work times (default)
                debug       Command execution log with timestamps

        mod                 List all timeline entries (work and break cycles) with numbers.

        mod start <op> <time>
                            Adjust the day start time (when work actually began).
            <op>            'add' (later start) or 'sub' (earlier start)
            <time>          Time in HHMM format
            Example: wt mod start sub 30     (started 30min earlier)

        mod <num> <op> <time>
                            Modify a specific cycle's duration.
            <num>           Cycle number from 'wt mod' list
            <op>            'add' or 'sub'
            <time>          Time in HHMM format
            Example: wt mod 3 add 15         (add 15min to cycle 3)

        mod <num> drop      Remove a cycle. If removing a break between work
                            cycles, they will merge. If removing a work cycle
                            between breaks, the breaks will merge.

        report              Print a one-line summary of the day's work including
                            start time, total work time, total break time, and end time.

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
    # Current cycle work minutes (already excludes paused time)
    current = calculate_current_minutes(timer)
    total = current + timer.completed_minutes()

    return hour_minute_str_from_minutes(total)


def hour_minute_to_minutes(hours: int, minutes: int) -> int:
    return hours * 60 + minutes


def calculate_current_minutes(timer: Timer) -> int:
    """Calculate work minutes for current cycle: total_elapsed - paused."""
    if timer.status == Status.Stopped or not timer.cycle_start_datetime_str:
        return 0
    
    # Total time since cycle started
    cycle_start_dt = dt.strptime(timer.cycle_start_datetime_str, DT_FORMAT)
    total_elapsed = delta_minutes(cycle_start_dt, get_current_time())
    
    if timer.status == Status.Paused:
        # Add current pause duration to total paused
        pause_start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
        current_pause = delta_minutes(pause_start_dt, get_current_time())
        total_paused = timer.paused_minutes + current_pause
    else:
        total_paused = timer.paused_minutes
    
    work_minutes = total_elapsed - total_paused
    return max(0, work_minutes)  # Ensure non-negative


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
        "day_start": timer.day_start,
        "cycle_start_datetime_str": timer.cycle_start_datetime_str,
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

    # Backward compatibility: support old 'accumulated_minutes' field
    paused = data.get("paused_minutes", data.get("accumulated_minutes", 0))

    return Timer(
        Status(data["status"]),
        data["start_datetime_str"],
        data["stop_datetime_str"],
        paused,
        data["mode"],
        data.get("timeline", []),
        data.get("day_start", ""),
        data.get("cycle_start_datetime_str", ""))


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


def daily_report_file_path() -> str:
    return f"{project_root_path()}/{DAILY_REPORT_PATH}"


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
    timestamp = get_current_time().strftime(DT_FORMAT)
    with open(debug_log_file_path(), "a") as file:
        file.write(f"[{timestamp}] {msg}\n")


def log_info(msg: str):
    with open(info_log_file_path(), "a") as file:
        file.write(f"{msg}\n")


if __name__ == "__main__":
    main()
