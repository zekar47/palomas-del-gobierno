# PALOMAS DEL GOBIERNO — Makefile de desarrollo
#
#   make dev      -> compila, arranca el binario y vigila cambios (principal)
#   make build    -> solo compila el binario
#   make start    -> arranca el binario en segundo plano (guarda PID en .dev.pid)
#   make stop     -> detiene el binario de desarrollo
#   make restart  -> reinicia sin recompilar (archivos estaticos: templates/, static/)
#   make rebuild  -> recompila + reinicia (archivos de codigo: *.go)
#   make watch    -> vigila cambios y decide rebuild vs restart
#   make run      -> corre en primer plano sin vigilar (una sola vez)
#   make clean    -> borra binario y PID
#
# Requiere: go + inotifywait (paquete inotify-tools).
#   Debian/Ubuntu: sudo apt install inotify-tools
#   Fedora:        sudo dnf install inotify-tools
#   Nix (este repo): nix develop  (o agrega inotify-tools al devShell)

BIN      := palomas
DEV_PID  := .dev.pid

ADDR     ?= :8080
DB       ?= palomas.db
UPLOADS  ?= uploads
ARGS     := --addr $(ADDR) --db $(DB) --uploads $(UPLOADS)

# inotifywait excluye ruido: .git, vendor, uploads, sqlite, binario, PID, result de nix
INOTIFY_EXCLUDE := (.*\.git/.*|.*/vendor/.*|.*/uploads/.*|.*\.db(-wal|-shm)?$$|.*/palomas$$|.*\.dev\.pid.*|.*/result.*)

.PHONY: dev build start stop restart rebuild watch run clean help

help:
	@echo "uso:"
	@echo "  make dev      compila, arranca y vigila (principal)"
	@echo "  make build    compila ./$(BIN)"
	@echo "  make start    arranca ./$(BIN) en segundo plano"
	@echo "  make stop     detiene el proceso de desarrollo"
	@echo "  make restart  reinicia sin recompilar (estaticos)"
	@echo "  make rebuild  recompila + reinicia (codigo)"
	@echo "  make watch    vigila cambios (lo usa 'make dev')"
	@echo "  make run      corre en primer plano, sin vigilar"
	@echo "  make clean    borra binario y PID"

build:
	go build -o $(BIN) .

run: build
	./$(BIN) $(ARGS)

start:
	@$(MAKE) stop >/dev/null 2>&1 || true
	@./$(BIN) $(ARGS) & echo $$! > $(DEV_PID)
	@echo "en linea (pid $$(cat $(DEV_PID)))"

stop:
	@if [ -f $(DEV_PID) ]; then \
		kill $$(cat $(DEV_PID)) 2>/dev/null || true; \
		rm -f $(DEV_PID); \
		echo "detenido"; \
	else \
		echo "nada corriendo"; \
	fi

# Estatico (templates/, static/): el binario sirve esos archivos en cada
# request (ParseFiles al arrancar para templates, FileServer para static),
# PERO las plantillas se cachean con loadTemplates() al arranque, asi que
# hay que reiniciar el proceso. No hace falta recompilar.
restart:
	@$(MAKE) stop
	@$(MAKE) start

# Codigo (*.go): hay que recompilar el binario y reiniciar.
rebuild:
	@$(MAKE) build
	@$(MAKE) restart

watch:
	@command -v inotifywait >/dev/null 2>&1 || { \
		echo "falta inotifywait (paquete inotify-tools)"; \
		echo "  Debian/Ubuntu: sudo apt install inotify-tools"; \
		echo "  Fedora:        sudo dnf install inotify-tools"; \
		exit 1; \
	}
	@echo "vigilando: *.go -> rebuild | templates/* static/* -> restart"
	@while file=$$(inotifywait -r \
		-e close_write -e modify -e moved_to -e create -e delete \
		--exclude '$(INOTIFY_EXCLUDE)' \
		--format '%w%f' . 2>/dev/null); do \
		case "$$file" in \
			*.go) \
				echo "=> codigo ($$file): recompilando y reiniciando"; \
				$(MAKE) rebuild ;; \
			*templates*|*static*|*.html|*.css|*.js|*.jpeg|*.jpg|*.png|*.webp|*.svg) \
				echo "=> estatico ($$file): reiniciando"; \
				$(MAKE) restart ;; \
			*) \
				echo "=> ignorado ($$file)" ;; \
		esac; \
	done

# Principal: compila, arranca y vigila. Llama a los demas comandos.
dev:
	@command -v inotifywait >/dev/null 2>&1 || { \
		echo "falta inotifywait (paquete inotify-tools)"; \
		echo "  Debian/Ubuntu: sudo apt install inotify-tools"; \
		echo "  Fedora:        sudo dnf install inotify-tools"; \
		exit 1; \
	}
	@$(MAKE) build
	@$(MAKE) start
	@trap '$(MAKE) stop' INT TERM EXIT; $(MAKE) watch

clean:
	@$(MAKE) stop >/dev/null 2>&1 || true
	rm -f $(BIN) $(DEV_PID)
