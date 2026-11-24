import { createBrowserRouter, Navigate } from "react-router-dom";
import UserLayout from "@/layouts/user-layout";
import CanvasPage from "@/pages/canvas";
import CanvasProjectPage from "@/pages/canvas/project";
export const router = createBrowserRouter([
  { path: "/", element: <Navigate to="/canvas" replace /> },
  { path: "/canvas", element: <UserLayout><CanvasPage /></UserLayout> },
  { path: "/canvas/:id", element: <UserLayout><CanvasProjectPage /></UserLayout> },
  { path: "*", element: <Navigate to="/canvas" replace /> },
], { basename: "/canvas" });
