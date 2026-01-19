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
            timeline=None,
            dayStart=""):
        self.status: Status = status
        self.start_datetime_str: str = start
        self.stop_datetime_str: str = stop
        self.paused_minutes: int = pausedTime
        self.mode: Mode = mode
        # Timeline entries: {"type": "work"|"break", "minutes": N}
        self.timeline: List[dict] = timeline if timeline is not None else []
        # When did this day's work start (first work cycle start time)
        self.day_start: str = dayStart

    def __str__(self):
        return (
            f"status = {self.status}\n"
            f"day_start = {self.day_start}\n"
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
        case Status.Stopped:
            message = "Starting timer."

    # Calculate break if resuming from stopped state
    if timer.stop_datetime_str != "":
        break_start_dt = dt.strptime(timer.stop_datetime_str, DT_FORMAT)
        break_stop_dt = dt.now()
        break_mins = delta_minutes(break_start_dt, break_stop_dt)
        timer.timeline.append({
            "type": "break",
            "minutes": break_mins
        })

    timer.stop_datetime_str = ""
    now = dt.now()
    timer.start_datetime_str = now.strftime(DT_FORMAT)
    
    # If this is the first cycle of the day, set day_start
    if not timer.day_start:
        timer.day_start = timer.start_datetime_str
    
    timer.status = Status.Running

    start_time_log = f" {start_time}" if start_time != None else ""
    log_debug(f"wt start{start_time_log}")

    save(timer)
    print_message_if_not_silent(timer, message)
    print_check_if_verbose(timer)

    # Handle backdating with start_time parameter
    if start_time != None:
        # Can only backdate when starting from stopped
        # This is used by restart command
        minutes = string_time_to_minutes(start_time)
        timer = load()
        start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
        new_start_dt = start_dt - timedelta(minutes=minutes)
        
        # Validate: new start must be before now
        if new_start_dt >= now:
            print(f"Cannot backdate start that far.")
            return
        
        timer.start_datetime_str = new_start_dt.strftime(DT_FORMAT)
        
        # If this was the first cycle, update day_start too
        if len(timer.timeline) == 0 or (len(timer.timeline) == 1 and timer.timeline[0]["type"] == "break"):
            timer.day_start = new_start_dt.strftime(DT_FORMAT)
        
        # Adjust the break duration if we just added one
        if timer.timeline and timer.timeline[-1]["type"] == "break":
            # Recalculate break: from stop_datetime_str to new start
            # But we need the actual stop time from before
            # Actually, the break was already calculated before backdating
            # We need to adjust it: the break should now be shorter
            break_entry = timer.timeline[-1]
            old_break_mins = break_entry["minutes"]
            # The break started at stop_datetime_str and originally ended at old start time
            # Now it ends at new_start_dt
            # So the new break duration is: old_break_mins - minutes
            new_break_mins = old_break_mins - minutes
            if new_break_mins <= 0:
                # Remove break entirely
                timer.timeline.pop()
            else:
                break_entry["minutes"] = new_break_mins
        
        save(timer)


def stop():
    timer = load()
    match timer.status:
        case Status.Stopped:
            print("Timer already stopped.")
        case Status.Running | Status.Paused:
            now = dt.now()
            stop_time_str = now.strftime(DT_FORMAT)
            
            # Calculate work duration
            if timer.status == Status.Paused:
                cycle_minutes = timer.paused_minutes
            else:
                start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
                cycle_minutes = delta_minutes(start_dt, now) + timer.paused_minutes
            
            # Add work entry to timeline
            timer.timeline.append({
                "type": "work",
                "minutes": cycle_minutes
            })
            
            timer.stop_datetime_str = stop_time_str
            timer.start_datetime_str = ""
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
                    start_dt += timedelta(minutes=timer.timeline[i]["minutes"])
                end_dt = start_dt + timedelta(minutes=cycle_minutes)
                
                start_time_only = start_dt.strftime(TIME_ONLY_FORMAT)
                end_time_only = end_dt.strftime(TIME_ONLY_FORMAT)
                
                # Check if dates differ
                day_diff = (end_dt.date() - start_dt.date()).days
                day_indicator = f"  [+{day_diff} day]" if day_diff > 0 else ""
                
                log_info(f"[{start_time_only} => {end_time_only}] Work: {cycle_str} ({total_str}){day_indicator}")
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
        
        current_str = mintues_to_hour_minute_str(current_minutes)
        total_str = mintues_to_hour_minute_str(total_minutes)
        
        # Calculate when this cycle started
        if timer.day_start:
            cycle_start_dt = dt.strptime(timer.day_start, DT_FORMAT)
            for entry in timer.timeline:
                cycle_start_dt += timedelta(minutes=entry["minutes"])
        else:
            cycle_start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT) if timer.start_datetime_str else dt.now()
        
        start_time_only = cycle_start_dt.strftime(TIME_ONLY_FORMAT)
        
        # Check if crossed midnight
        now = dt.now()
        day_diff = (now.date() - cycle_start_dt.date()).days
        day_indicator = f"  [+{day_diff} day]" if day_diff > 0 else ""
        
        if timer.status == Status.Running:
            print(f"[{start_time_only} => .....] Work: {current_str} ({total_str}){day_indicator}")
        elif timer.status == Status.Paused:
            print(f"[{start_time_only} => .....] Work (paused): {current_str} ({total_str}){day_indicator}")


def report():
    """Print a one-line summary of the day's work."""
    timer = load()
    
    if not timer.day_start:
        print("No work recorded today.")
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
    
    # Add all timeline entries
    for entry in timer.timeline:
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
    
    print(f"{date_str} | {start_time} -> {end_time} | Work: {work_str} | Break: {break_str} | Total: {total_str}{day_indicator}")


def mod_list():
    """List all timeline entries with numbers."""
    timer = load()
    
    if not timer.timeline:
        print("No cycles to modify.")
        return
    
    # Calculate times from day_start
    if timer.day_start:
        current_time = dt.strptime(timer.day_start, DT_FORMAT)
    else:
        current_time = dt.now()
    
    running_total_work = 0
    
    for i, entry in enumerate(timer.timeline, 1):
        duration_mins = entry["minutes"]
        duration_str = mintues_to_hour_minute_str(duration_mins)
        
        # Calculate start and stop times
        start_time = current_time.strftime(TIME_ONLY_FORMAT)
        current_time += timedelta(minutes=duration_mins)
        stop_time = current_time.strftime(TIME_ONLY_FORMAT)
        
        # For work cycles, show running total
        if entry["type"] == "work":
            running_total_work += duration_mins
            total_str = mintues_to_hour_minute_str(running_total_work)
            print(f"{i:02d}. [{start_time} => {stop_time}] Work: {duration_str} ({total_str})")
        else:
            print(f"{i:02d}. [{start_time} => {stop_time}] Break: {duration_str}")
    
    # Print usage info
    print("\nUsage:")
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
    
    validate_timestring_or_quit(time_str)
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
    
    validate_timestring_or_quit(time_str)
    minutes = string_time_to_minutes(time_str)
    
    # Get the entry to modify (0-indexed)
    entry_idx = cycle_num - 1
    entry = timer.timeline[entry_idx]
    
    # Modify the duration
    if operation == "add":
        entry["minutes"] += minutes
    else:  # sub
        new_duration = entry["minutes"] - minutes
        if new_duration < 0:
            print(f"Error: Duration would be negative. Current: {mintues_to_hour_minute_str(entry['minutes'])}")
            return
        entry["minutes"] = new_duration
    
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
        current_time = dt.now()
    
    running_total = 0
    
    for entry in timer.timeline:
        if entry["type"] == "work":
            start_dt = current_time
            duration_mins = entry["minutes"]
            current_time += timedelta(minutes=duration_mins)
            stop_dt = current_time
            
            running_total += duration_mins
            
            start_time_only = start_dt.strftime(TIME_ONLY_FORMAT)
            end_time_only = stop_dt.strftime(TIME_ONLY_FORMAT)
            cycle_str = mintues_to_hour_minute_str(duration_mins)
            total_str = mintues_to_hour_minute_str(running_total)
            
            # Check if dates differ
            day_diff = (stop_dt.date() - start_dt.date()).days
            day_indicator = f"  [+{day_diff} day]" if day_diff > 0 else ""
            
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


def add(time: str):
    timer = load()
    validate_timestring_or_quit(time)
    minutes = string_time_to_minutes(time)

    if timer.status == Status.Stopped:
        # When stopped, add to the last work cycle duration
        if not timer.timeline or timer.timeline[-1]["type"] != "work":
            print("Cannot add time when stopped with no work cycles.")
            return
        
        timer.timeline[-1]["minutes"] += minutes
        
        # Regenerate info-log to reflect the change
        regenerate_info_log(timer)
        
        log_debug(f"wt add {time}")
        save(timer)
        return

    # For running/paused: backdate start (or day_start if first cycle)
    if timer.status == Status.Running:
        # If this is the first work cycle (no work entries in timeline yet)
        # backdate day_start instead
        has_work_cycles = any(entry["type"] == "work" for entry in timer.timeline)
        
        if not has_work_cycles:
            # First cycle - backdate day_start
            day_start_dt = dt.strptime(timer.day_start, DT_FORMAT)
            new_day_start = day_start_dt - timedelta(minutes=minutes)
            
            # Validate: must be before now
            if new_day_start >= dt.now():
                print("Cannot add that much time.")
                return
            
            timer.day_start = new_day_start.strftime(DT_FORMAT)
            
            # Also backdate start_datetime_str
            start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
            timer.start_datetime_str = (start_dt - timedelta(minutes=minutes)).strftime(DT_FORMAT)
            
            # Adjust break duration if there is one
            if timer.timeline and timer.timeline[-1]["type"] == "break":
                # Break is now longer by `minutes`
                timer.timeline[-1]["minutes"] += minutes
        else:
            # Not first cycle - just backdate current start
            start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
            new_start_dt = start_dt - timedelta(minutes=minutes)
            
            # Validate: new start must be before now
            if new_start_dt >= dt.now():
                print(f"Cannot add that much time.")
                return
            
            timer.start_datetime_str = new_start_dt.strftime(DT_FORMAT)
            
            # Adjust the last break if there is one
            if timer.timeline and timer.timeline[-1]["type"] == "break":
                # The break is now longer by `minutes`
                timer.timeline[-1]["minutes"] += minutes
        
        log_debug(f"wt add {time}")
        save(timer)
    elif timer.status == Status.Paused:
        timer.paused_minutes += minutes
        log_debug(f"wt add {time}")
        save(timer)


def sub(time: str):
    timer = load()
    validate_timestring_or_quit(time)
    minutes = string_time_to_minutes(time)

    if timer.status == Status.Stopped:
        # When stopped, subtract from the last work cycle duration
        if not timer.timeline or timer.timeline[-1]["type"] != "work":
            print("Cannot subtract time when stopped with no work cycles.")
            return
        
        last_work = timer.timeline[-1]
        if last_work["minutes"] < minutes:
            print(f"Cannot reduce work cycle to below 0 minutes. Current: {mintues_to_hour_minute_str(last_work['minutes'])}")
            return
        
        last_work["minutes"] -= minutes
        
        # Regenerate info-log to reflect the change
        regenerate_info_log(timer)
        
        log_debug(f"wt sub {time}")
        save(timer)
        return

    # For running/paused: validate and subtract
    current_minutes = calculate_current_minutes(timer)
    if current_minutes < minutes:
        print("Cannot reduce current minutes to below 0.")
        return

    if timer.status == Status.Running:
        start_dt = dt.strptime(timer.start_datetime_str, DT_FORMAT)
        new_start_dt = start_dt + timedelta(minutes=minutes)
        
        # Validate: new start must still be before now
        if new_start_dt >= dt.now():
            print(f"Cannot subtract that much time.")
            return
        
        timer.start_datetime_str = new_start_dt.strftime(DT_FORMAT)
    elif timer.status == Status.Paused:
        timer.paused_minutes -= minutes

    log_debug(f"wt sub {time}")
    save(timer)


def next_timer():
    stop()

    # Add a 0-minute break between cycles
    timer = load()
    timer.timeline.append({
        "type": "break",
        "minutes": 0
    })
    save(timer)

    # Start next cycle (skip break calculation since we just added one)
    timer.stop_datetime_str = ""
    now = dt.now()
    timer.start_datetime_str = now.strftime(DT_FORMAT)
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
    
    # Add all timeline entries
    for entry in timer.timeline:
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
    
    # Append to daily report file
    with open(daily_report_file_path(), "a") as file:
        file.write(report_line)


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
        "day_start": timer.day_start,
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
        data.get("timeline", []),
        data.get("day_start", ""))


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
    timestamp = dt.now().strftime(DT_FORMAT)
    with open(debug_log_file_path(), "a") as file:
        file.write(f"[{timestamp}] {msg}\n")


def log_info(msg: str):
    with open(info_log_file_path(), "a") as file:
        file.write(f"{msg}\n")


if __name__ == "__main__":
    main()
