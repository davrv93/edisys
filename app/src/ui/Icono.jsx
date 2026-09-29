// Iconos de Lucide (https://lucide.dev), uno por concepto y el mismo en toda la app (plan §2, bloque C).
// Importación nombrada: el build solo lleva los iconos de esta lista.
// Se piden por su nombre en español (API de siempre: <Icono nombre="recibo" />); también se acepta
// el nombre de Lucide de los iconos registrados aquí (p. ej. «Landmark», «FileCheck2»).
// Siempre acompañan a un texto o llevan aria-label en su botón: nunca son el único portador del significado.
import {
  Archive, ArrowDownLeft, ArrowDownRight, ArrowLeft, ArrowUpRight, BadgeCheck, Ban, Bot, Building2,
  CalendarCheck, CalendarClock, CalendarDays, Camera, ChartColumn, Check, CheckCheck, ChevronDown, ChevronLeft,
  ChevronRight, ChevronUp, Circle, CircleAlert, CircleCheck, CircleDashed, CircleDot, CircleOff, CircleX, Clock,
  Download, Ellipsis, Eye, FilePen, FileSpreadsheet, FileText, FlaskConical, Gauge,
  GripVertical, Hourglass, Inbox, Info, Landmark, LayoutDashboard, Loader, Lock, LogOut,
  Mail, Menu, MessageCircle, Minus, PanelLeftClose, PanelLeftOpen, Plus, Printer, ReceiptText, Scale, Search,
  Send, Settings, ShieldCheck, SlidersHorizontal, Timer, TrendingDown, TrendingUp, TriangleAlert, Upload,
  User, UserX, Vote, Wrench, X,
} from 'lucide-react';

/** Concepto (en español) → icono de Lucide. Los nombres de la v1 se conservan. */
export const ICONOS = {
  // Secciones (menú)
  inicio: LayoutDashboard,
  resumen: LayoutDashboard,
  portal: LayoutDashboard,
  balance: Scale,
  recibo: ReceiptText,
  edificio: Building2,
  calendario: CalendarCheck,
  calendario_dias: CalendarDays,
  medidor: Gauge,
  herramienta: Wrench,
  whatsapp: MessageCircle,
  mensaje: MessageCircle,
  robot: Bot,
  chatbot: Bot,
  grafico: ChartColumn,
  llave: ShieldCheck,
  roles: ShieldCheck,
  conciliacion: Landmark, // aviso de conciliación en el balance
  // Objetos y acciones
  camara: Camera,
  voucher: BadgeCheck,
  moroso: CircleAlert,
  alerta: TriangleAlert,
  critico: TriangleAlert,
  junta: Vote,
  votos: Vote,
  arrastrar: GripVertical,
  excel: FileSpreadsheet,
  subir: Upload,
  descargar: Download,
  imprimir: Printer,
  enviar: Send,
  documento: FileText,
  usuario: User,
  candado: Lock,
  correo: Mail,
  ver: Eye,
  engranaje: Settings,
  ajustes: SlidersHorizontal,
  buscar: Search,
  // Navegación y controles
  menu: Menu,
  mas: Ellipsis,
  mas_signo: Plus,
  menos: Minus,
  salir: LogOut,
  volver: ArrowLeft,
  izq: ChevronLeft,
  der: ChevronRight,
  abajo: ChevronDown,
  arriba: ChevronUp,
  sube: ArrowUpRight,
  baja: ArrowDownRight,
  entrante: ArrowDownLeft,
  cerrar: X,
  check: Check,
  info: Info,
  plegar: PanelLeftClose,
  desplegar: PanelLeftOpen,
  // Estados (los usa ui/estados.js)
  reloj: Clock,
  reloj_arena: Hourglass,
  temporizador: Timer,
  cargando: Loader,
  hecho: CircleCheck,
  pagado: BadgeCheck,
  doble_check: CheckCheck,
  parcial: CircleDashed,
  borrador: FilePen,
  anulado: Ban,
  cancelado: CircleX,
  no_show: UserX,
  inactivo: CircleOff,
  punto: CircleDot,
  circulo: Circle,
  archivado: Archive,
  bandeja: Inbox,
  simulado: FlaskConical,
  pico: TrendingUp,
  negativo: TrendingDown,
  agenda: CalendarClock,
};

// Los mismos componentes, también por su nombre de Lucide («Wrench», «Vote»…).
const POR_NOMBRE_LUCIDE = { Archive, ArrowDownLeft, ArrowDownRight, ArrowLeft, ArrowUpRight, BadgeCheck, Ban, Bot, Building2, CalendarCheck, CalendarClock, CalendarDays, Camera, ChartColumn, Check, CheckCheck, ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Circle, CircleAlert, CircleCheck, CircleDashed, CircleDot, CircleOff, CircleX, Clock, Download, Ellipsis, Eye, FilePen, FileSpreadsheet, FileText, FlaskConical, Gauge, GripVertical, Hourglass, Inbox, Info, Landmark, LayoutDashboard, Loader, Lock, LogOut, Mail, Menu, MessageCircle, Minus, PanelLeftClose, PanelLeftOpen, Plus, Printer, ReceiptText, Scale, Search, Send, Settings, ShieldCheck, SlidersHorizontal, Timer, TrendingDown, TrendingUp, TriangleAlert, Upload, User, UserX, Vote, Wrench, X };

/** ¿Existe el icono? (acepta el nombre en español o el de Lucide). */
export function existeIcono(nombre) {
  return !!(ICONOS[nombre] || POR_NOMBRE_LUCIDE[nombre]);
}

/**
 * Icono de trazo. Tamaños del sistema: 16 / 18 / 20 (el 24+ solo en estados vacíos y la cámara).
 * `titulo` lo vuelve imagen con nombre; sin título es decorativo (aria-hidden).
 */
export default function Icono({ nombre, tam = 20, className = '', grosor = 1.75, titulo }) {
  const C = ICONOS[nombre] || POR_NOMBRE_LUCIDE[nombre] || Info;
  return (
    <C
      size={tam}
      strokeWidth={grosor}
      absoluteStrokeWidth={false}
      className={'shrink-0 ' + className}
      aria-hidden={titulo ? undefined : 'true'}
      role={titulo ? 'img' : undefined}
      aria-label={titulo || undefined}
      focusable="false"
    />
  );
}
