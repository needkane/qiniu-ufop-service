package ufop

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	HTML2PDF_MAX_PAGE_SIZE = 10 * 1024 * 1024
	HTML2PDF_MAX_COPIES    = 10
)

type Html2Pdfer struct {
	maxPageSize int64
	maxCopies   int
}

type Html2PdfOptions struct {
	Gray        bool
	LowQuality  bool
	Orientation string
	Size        string
	Title       string
	Collate     bool
	Copies      int
}

func (this *Html2Pdfer) parse(cmd string) (options *Html2PdfOptions, err error) {
	pattern := `^html2pdf(/gray/[0|1]|/low/[0|1]|/orient/(Portrait|Landscape)|/size/[A-B][0-8]|/title/[0-9a-zA-Z-_=]+|/collate/[0|1]|/copies/\d+)*$`
	matched, _ := regexp.Match(pattern, []byte(cmd))
	if !matched {
		err = errors.New("invalid html2pdf command format")
		return
	}

	var decodeErr error

	//get optional parameters

	options = &Html2PdfOptions{
		Collate: true,
		Copies:  1,
	}

	//get gray
	grayStr := getParam(cmd, "gray/[0|1]", "gray")
	if grayStr != "" {
		grayInt, _ := strconv.Atoi(grayStr)
		if grayInt == 1 {
			options.Gray = true
		}
	}

	//get low quality
	lowStr := getParam(cmd, "low/[0|1]", "low")
	if lowStr != "" {
		lowInt, _ := strconv.Atoi(lowStr)
		if lowInt == 1 {
			options.LowQuality = true
		}
	}

	//orient
	options.Orientation = getParam(cmd, "orient/(Portrait|Landscape)", "orient")

	//size
	options.Size = getParam(cmd, "size/[A-B][0-8]", "size")

	//title
	title, decodeErr := getParamDecoded(cmd, "title/[0-9a-zA-Z-_=]+", "title")
	if decodeErr != nil {
		err = errors.New("invalid html2pdf parameter 'title'")
		return
	}
	options.Title = title

	//collate
	collateStr := getParam(cmd, "collate/[0|1]", "collate")
	if collateStr != "" {
		collateInt, _ := strconv.Atoi(collateStr)
		if collateInt == 0 {
			options.Collate = false
		}
	}

	//copies
	copiesStr := getParam(cmd, `copies/\d+`, "copies")
	if copiesStr != "" {
		copiesInt, _ := strconv.Atoi(copiesStr)
		if copiesInt <= 0 {
			err = errors.New("invalid html2pdf parameter 'copies'")
		} else {
			options.Copies = copiesInt
		}
	}

	return
}

func (this *Html2Pdfer) Do(req UfopRequest) (result interface{}, contentType string, err error) {
	if this.maxPageSize <= 0 {
		this.maxPageSize = HTML2PDF_MAX_PAGE_SIZE
	}

	if this.maxCopies <= 0 {
		this.maxCopies = HTML2PDF_MAX_COPIES
	}

	//if not text format, error it
	if !strings.HasPrefix(req.Src.MimeType, "text/") {
		err = errors.New("unsupported file mime type, only text/* allowed")
		return
	}

	//if file size exceeds, error it
	if req.Src.Fsize > this.maxPageSize {
		err = errors.New("page file length exceeds the limit")
		return
	}

	options, pErr := this.parse(req.Cmd)
	if pErr != nil {
		err = pErr
		return
	}

	if options.Copies > this.maxCopies {
		err = errors.New("pdf copies exceeds the limit")
		return
	}

	//html2pdf options
	cmdParams := make([]string, 0)
	cmdParams = append(cmdParams, "-q")
	if options.Gray {
		cmdParams = append(cmdParams, "--grayscale")
	}

	if options.LowQuality {
		cmdParams = append(cmdParams, "--lowquality")
	}

	if options.Orientation != "" {
		cmdParams = append(cmdParams, "--orientation", options.Orientation)
	}

	if options.Size != "" {
		cmdParams = append(cmdParams, "--page-size", options.Size)
	}

	if options.Title != "" {
		cmdParams = append(cmdParams, "--title", options.Title)
	}

	if options.Collate {
		cmdParams = append(cmdParams, "--collate")
	} else {
		cmdParams = append(cmdParams, "--no-collate")
	}

	cmdParams = append(cmdParams, "--copies", fmt.Sprintf("%d", options.Copies))

	//result tmp file
	resultTmpFname := fmt.Sprintf("%s%d.pdf", md5Hex(req.Src.Url), time.Now().UnixNano())
	defer os.Remove(resultTmpFname)

	cmdParams = append(cmdParams, req.Src.Url, resultTmpFname)

	//cmd
	convertCmd := exec.Command("wkhtmltopdf", cmdParams...)
	log.Println(convertCmd.Path, convertCmd.Args)

	stdOutPipe, pipeErr := convertCmd.StdoutPipe()
	if pipeErr != nil {
		err = errors.New(fmt.Sprintf("open exec stdout pipe error, %s", pipeErr.Error()))
		return
	}

	if startErr := convertCmd.Start(); startErr != nil {
		err = errors.New(fmt.Sprintf("start html2pdf command error, %s", startErr.Error()))
		return
	}

	stdErrData, readErr := ioutil.ReadAll(stdOutPipe)
	if readErr != nil {
		err = errors.New(fmt.Sprintf("read html2pdf command stdout error, %s", readErr.Error()))
		return
	}

	if waitErr := convertCmd.Wait(); waitErr != nil {
		err = errors.New(fmt.Sprintf("wait html2pdf to exit error, %s", waitErr))
		return
	}

	//check stdout output & output file
	if string(stdErrData) != "" {
		log.Println(string(stdErrData))
	}

	if _, statErr := os.Stat(resultTmpFname); statErr == nil {
		oTmpFp, openErr := os.Open(resultTmpFname)
		if openErr != nil {
			err = errors.New(fmt.Sprintf("open html2pdf output result error, %s", openErr.Error()))
			return
		}
		defer oTmpFp.Close()

		outputBytes, readErr := ioutil.ReadAll(oTmpFp)
		if readErr != nil {
			err = errors.New(fmt.Sprintf("read html2pdf output result error, %s", readErr.Error()))
			return
		}
		result = outputBytes
	}

	contentType = "application/pdf"
	return
}