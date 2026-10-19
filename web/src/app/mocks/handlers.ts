/** MSW handler registry — ONE line per module (`modules/<m>/infrastructure/mocks.ts` exports `handlers`). */
import type { HttpHandler } from "msw";
import { handlers as auth } from "@/modules/auth/infrastructure/mocks";
import { handlers as dashboard } from "@/modules/dashboard/infrastructure/mocks";
import { handlers as campaign } from "@/modules/campaign/infrastructure/mocks";
import { handlers as brand } from "@/modules/brand/infrastructure/mocks";
import { handlers as content } from "@/modules/content/infrastructure/mocks";
import { handlers as image } from "@/modules/image/infrastructure/mocks";
import { handlers as video } from "@/modules/video/infrastructure/mocks";
import { handlers as library } from "@/modules/library/infrastructure/mocks";
import { handlers as review } from "@/modules/review/infrastructure/mocks";
import { handlers as job } from "@/modules/job/infrastructure/mocks";
import { handlers as budget } from "@/modules/budget/infrastructure/mocks";
import { handlers as settings } from "@/modules/settings/infrastructure/mocks";

export const handlers: HttpHandler[] = [
  ...auth,
  ...dashboard,
  ...campaign,
  ...brand,
  ...content,
  ...image,
  ...video,
  ...library,
  ...review,
  ...job,
  ...budget,
  ...settings,
];
