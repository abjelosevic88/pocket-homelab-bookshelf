// Languages the catalogues can filter by (ISO 639-1 → English name).
export const LANGUAGES: Record<string, string> = {
  en: 'English', sr: 'Serbian', hr: 'Croatian', bs: 'Bosnian', sl: 'Slovenian', mk: 'Macedonian', de: 'German', fr: 'French',
  es: 'Spanish', it: 'Italian', pt: 'Portuguese', ru: 'Russian', hu: 'Hungarian', pl: 'Polish', cs: 'Czech', sk: 'Slovak',
  nl: 'Dutch', sv: 'Swedish', no: 'Norwegian', da: 'Danish', fi: 'Finnish', ro: 'Romanian', bg: 'Bulgarian', el: 'Greek',
  tr: 'Turkish', uk: 'Ukrainian', ja: 'Japanese', zh: 'Chinese', ko: 'Korean', ar: 'Arabic', he: 'Hebrew',
}
export function langName(code: string): string { return LANGUAGES[code] || code }
