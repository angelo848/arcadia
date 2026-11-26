// Pure logic of the session form (phase 16, SEL-005..SEL-009, SEL-016).

export type SessionFormValues = {
  start: string;
  end: string;
  hours: string;
  minutes: string;
  includedInTotal: boolean;
};

export type SessionFormState = {
  values: SessionFormValues;
  initial: SessionFormValues;
  activeFollowsSpan: boolean;
};

export type EditReason =
  | "end-not-after-start"
  | "in-future"
  | "too-early"
  | "span-too-long"
  | "active-too-short"
  | "active-exceeds-span"
  | "discarded"
  | "exceeds-adjustment"
  | "empty-patch";

export const sessionReasonMessages: Record<EditReason, string> = {
  "end-not-after-start": "O fim precisa ser depois do início.",
  "in-future": "O fim não pode estar no futuro.",
  "too-early": "O início não pode ser antes de 1980.",
  "span-too-long": "Uma sessão pode ter no máximo 24 horas.",
  "active-too-short": "O tempo jogado precisa ter pelo menos 1 minuto.",
  "active-exceeds-span":
    "O tempo jogado não pode passar do intervalo entre início e fim.",
  discarded: "Sessões descartadas não podem ser editadas, só excluídas.",
  "exceeds-adjustment":
    "O tempo jogado é maior que a parte do tempo total que não vem de sessões.",
  "empty-patch": "Nada para salvar.",
};

const emptyMessage = "Preencha este campo.";
const notNumberMessage = "Informe um número válido.";
const minutesMessage = "Os minutos vão de 0 a 59.";

const hourMs = 3_600_000;
const minuteMs = 60_000;
const year1980 = Date.parse("1980-01-01T00:00:00Z");

export type SessionFormErrors = {
  start?: string;
  end?: string;
  active?: string;
};

const pad = (n: number) => String(n).padStart(2, "0");

// utcIsoToLocal formats an instant as the local value of a datetime-local input.
export function utcIsoToLocal(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return "";
  }
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// localToUtcIso converts the local value of an input to UTC; the browser's Date resolves DST gaps and overlaps (SEL-016).
export function localToUtcIso(local: string): string | null {
  if (local === "") {
    return null;
  }
  const d = new Date(local);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

function localMs(local: string): number | null {
  const iso = localToUtcIso(local);
  return iso === null ? null : Date.parse(iso);
}

function spanMinutes(v: SessionFormValues): number | null {
  const s = localMs(v.start);
  const e = localMs(v.end);
  if (s === null || e === null) {
    return null;
  }
  return Math.max(0, Math.floor((e - s) / minuteMs));
}

function partsOf(totalMinutes: number): { hours: string; minutes: string } {
  return {
    hours: String(Math.floor(totalMinutes / 60)),
    minutes: String(totalMinutes % 60),
  };
}

// typedMinutes reads the typed active time in minutes; null when it is not a valid number.
function typedMinutes(hours: string, minutes: string): number | null {
  if (!/^\d+$/.test(hours.trim()) || !/^\d+$/.test(minutes.trim())) {
    return null;
  }
  return Number(hours) * 60 + Number(minutes);
}

export function createFormState(now: Date): SessionFormState {
  const endMs = Math.floor(now.getTime() / minuteMs) * minuteMs;
  const values: SessionFormValues = {
    start: utcIsoToLocal(new Date(endMs - hourMs).toISOString()),
    end: utcIsoToLocal(new Date(endMs).toISOString()),
    hours: "1",
    minutes: "0",
    includedInTotal: false,
  };
  return { values, initial: { ...values }, activeFollowsSpan: true };
}

export function editFormState(item: {
  startedAt: string;
  endedAt: string | null;
  activeSeconds: number;
}): SessionFormState {
  const activeMin = Math.floor(item.activeSeconds / 60);
  const values: SessionFormValues = {
    start: utcIsoToLocal(item.startedAt),
    end: item.endedAt ? utcIsoToLocal(item.endedAt) : "",
    ...partsOf(activeMin),
    includedInTotal: false,
  };
  return {
    values,
    initial: { ...values },
    activeFollowsSpan: spanMinutes(values) === activeMin,
  };
}

export function changeField(
  state: SessionFormState,
  field: keyof SessionFormValues,
  value: string | boolean,
): SessionFormState {
  const values = { ...state.values, [field]: value } as SessionFormValues;
  let follows = state.activeFollowsSpan;
  if (field === "start" || field === "end") {
    const span = spanMinutes(values);
    if (follows && span !== null) {
      Object.assign(values, partsOf(span));
    }
  } else if (field === "hours" || field === "minutes") {
    const span = spanMinutes(values);
    follows =
      span !== null && typedMinutes(values.hours, values.minutes) === span;
  }
  return { ...state, values, activeFollowsSpan: follows };
}

// withComputedEnd fills the end of a new session as start + active time (the form does not ask for it).
export function withComputedEnd(v: SessionFormValues): SessionFormValues {
  const s = localMs(v.start);
  const active = typedMinutes(v.hours, v.minutes);
  if (s === null || active === null) {
    return { ...v, end: "" };
  }
  return {
    ...v,
    end: utcIsoToLocal(new Date(s + active * minuteMs).toISOString()),
  };
}

export function validateSessionForm(
  v: SessionFormValues,
  ctx: { now: Date; source: string; adjustmentSeconds: number | null },
): SessionFormErrors {
  const errors: SessionFormErrors = {};
  const s = localMs(v.start);
  const e = localMs(v.end);
  if (v.start.trim() === "") {
    errors.start = emptyMessage;
  } else if (s === null) {
    errors.start = notNumberMessage;
  }
  if (v.end.trim() === "") {
    errors.end = emptyMessage;
  } else if (e === null) {
    errors.end = notNumberMessage;
  }
  if (s !== null && e !== null) {
    if (s < year1980) {
      errors.start = sessionReasonMessages["too-early"];
    }
    if (e <= s) {
      errors.end = sessionReasonMessages["end-not-after-start"];
    } else if (e > ctx.now.getTime()) {
      errors.end = sessionReasonMessages["in-future"];
    } else if (ctx.source !== "import" && e - s > 24 * hourMs) {
      errors.end = sessionReasonMessages["span-too-long"];
    }
  }

  if (v.hours.trim() === "" || v.minutes.trim() === "") {
    errors.active = emptyMessage;
  } else if (!/^\d+$/.test(v.hours.trim()) || !/^\d+$/.test(v.minutes.trim())) {
    errors.active = notNumberMessage;
  } else if (Number(v.minutes) > 59) {
    errors.active = minutesMessage;
  } else {
    const activeMs = (Number(v.hours) * 60 + Number(v.minutes)) * minuteMs;
    if (activeMs < minuteMs) {
      errors.active = sessionReasonMessages["active-too-short"];
    } else if (s !== null && e !== null && e > s && activeMs > e - s) {
      errors.active = sessionReasonMessages["active-exceeds-span"];
    } else if (
      v.includedInTotal &&
      ctx.adjustmentSeconds !== null &&
      activeMs / 1000 > ctx.adjustmentSeconds
    ) {
      errors.active = sessionReasonMessages["exceeds-adjustment"];
    }
  }
  return errors;
}

type TotalsView = { totalPlaySeconds: number; sessionPlaySeconds: number };

// manualAdjustmentSeconds is the part of the total play time that does not come from sessions.
export function manualAdjustmentSeconds(g: TotalsView): number {
  return Math.max(0, g.totalPlaySeconds - g.sessionPlaySeconds);
}

// showIncludedInTotal: the box is offered only when creating and with a positive adjustment (SEL-009).
export function showIncludedInTotal(
  mode: "create" | "edit",
  g: TotalsView,
): boolean {
  return mode === "create" && manualAdjustmentSeconds(g) > 0;
}

function activeSecondsOf(v: SessionFormValues): number {
  return (Number(v.hours) * 60 + Number(v.minutes)) * 60;
}

export type SessionCreateInput = {
  startedAt: string;
  endedAt: string;
  activeSeconds: number;
  includedInTotal: boolean;
};

export function toCreateInput(v: SessionFormValues): SessionCreateInput {
  return {
    startedAt: localToUtcIso(v.start) ?? "",
    endedAt: localToUtcIso(v.end) ?? "",
    activeSeconds: activeSecondsOf(v),
    includedInTotal: v.includedInTotal,
  };
}

export type SessionUpdatePatch = {
  startedAt?: string;
  endedAt?: string;
  activeSeconds?: number;
};

// toUpdatePatch sends only what changed; an untouched active time keeps the seconds of the tracked session.
export function toUpdatePatch(
  initial: SessionFormValues,
  current: SessionFormValues,
  _originalActiveSeconds: number,
): SessionUpdatePatch {
  const patch: SessionUpdatePatch = {};
  if (current.start !== initial.start) {
    patch.startedAt = localToUtcIso(current.start) ?? "";
  }
  if (current.end !== initial.end) {
    patch.endedAt = localToUtcIso(current.end) ?? "";
  }
  if (current.hours !== initial.hours || current.minutes !== initial.minutes) {
    patch.activeSeconds = activeSecondsOf(current);
  }
  return patch;
}
