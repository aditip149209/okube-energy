# Build the report and slides. The tables, plot data and headline numbers are
# generated from results/sim, models/artifacts and the experiment CSVs, so run
# `make data` after re-running `okube simulate` or retraining the power model.
.PHONY: all data clean

all: report.pdf slides.pdf

data:
	python3 -I make_data.py

report.pdf: report.tex generated/numbers.tex figures/architecture.tex
	pdflatex -interaction=nonstopmode -halt-on-error report.tex
	pdflatex -interaction=nonstopmode -halt-on-error report.tex

slides.pdf: slides.tex generated/numbers.tex figures/architecture_slide.tex
	pdflatex -interaction=nonstopmode -halt-on-error slides.tex
	pdflatex -interaction=nonstopmode -halt-on-error slides.tex

clean:
	rm -f *.aux *.log *.out *.nav *.snm *.toc *.vrb
