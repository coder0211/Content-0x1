/**
 * Module registry — ONE line per module. Each `modules/<m>/index.ts` exports any of
 * `routes`, `publicRoutes`, `navItems`, `repositories` (see shared/lib/module.ts → AppModule).
 */
import type { AppModule } from "@/shared/lib/module";
import * as auth from "@/modules/auth";
import * as dashboard from "@/modules/dashboard";
import * as campaign from "@/modules/campaign";
import * as brand from "@/modules/brand";
import * as content from "@/modules/content";
import * as image from "@/modules/image";
import * as video from "@/modules/video";
import * as library from "@/modules/library";
import * as review from "@/modules/review";
import * as job from "@/modules/job";
import * as budget from "@/modules/budget";
import * as settings from "@/modules/settings";

export const appModules: readonly AppModule[] = [
  auth,
  dashboard,
  campaign,
  brand,
  content,
  image,
  video,
  library,
  review,
  job,
  budget,
  settings,
];
