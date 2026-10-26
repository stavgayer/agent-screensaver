import { createTaskRowComponent } from "../shared/tasks.ts";
import { PULSE_MS, STYLE } from "./style.ts";

window.agentScreensaver.register(() => createTaskRowComponent({ css: STYLE, pulseMs: PULSE_MS }));
